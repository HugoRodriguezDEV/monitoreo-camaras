package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/alertas"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/go2rtc"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/marcas"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/salud"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/servidor"
)

func main() {
	ctx, fin := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer fin()
	if err := ejecutar(ctx, os.Args[1:]); err != nil {
		log.Print(err)
		fin()
		os.Exit(1)
	}
}

func ejecutar(ctx context.Context, args []string) error {
	const uso = "Uso: monitor run|check|go2rtc [-config monitor.yaml]"
	if len(args) == 0 {
		return errors.New(uso)
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Println(uso)
		return nil
	case "run", "check", "go2rtc":
	default:
		return fmt.Errorf("subcomando desconocido %q\n%s", args[0], uso)
	}

	opciones := flag.NewFlagSet(args[0], flag.ContinueOnError)
	opciones.SetOutput(io.Discard)
	ruta := opciones.String("config", "monitor.yaml", "Ruta de configuración")
	var mostrar bool
	if args[0] == "check" {
		opciones.BoolVar(&mostrar, "mostrar-password", false, "Mostrar las contraseñas")
	}
	if err := opciones.Parse(args[1:]); errors.Is(err, flag.ErrHelp) {
		fmt.Println(uso)
		if args[0] == "check" {
			fmt.Println("check admite -mostrar-password para mostrar las contraseñas.")
		}
		return nil
	} else if err != nil || opciones.NArg() != 0 {
		return fmt.Errorf("opciones inválidas\n%s", uso)
	}
	cfg, err := config.Cargar(*ruta)
	if err != nil {
		return err
	}
	switch args[0] {
	case "check":
		comprobar(ctx, cfg, mostrar)
		return nil
	case "go2rtc":
		datos, err := go2rtc.Generar(cfg)
		if err != nil {
			return fmt.Errorf("no se pudo generar la configuración de go2rtc")
		}
		if _, err := os.Stdout.Write(datos); err != nil {
			return fmt.Errorf("no se pudo imprimir la configuración de go2rtc")
		}
		return nil
	default:
		return iniciar(ctx, cfg)
	}
}

func comprobar(ctx context.Context, cfg config.Config, mostrar bool) {
	alcanzables := make([]bool, len(cfg.Sucursales))
	var grupo sync.WaitGroup
	for i, suc := range cfg.Sucursales {
		grupo.Go(func() {
			_, err := salud.SondearTCP(ctx, suc.DVR.Direccion())
			alcanzables[i] = err == nil
		})
	}
	grupo.Wait()
	for i, suc := range cfg.Sucursales {
		alcance := "no"
		if alcanzables[i] {
			alcance = "sí"
		}
		fmt.Printf("%s (%s) · marca %s · alcanzable: %s\n", suc.ID, suc.Nombre, suc.DVR.Marca, alcance)
		for _, c := range suc.Camaras {
			for _, hd := range []bool{false, true} {
				direccion := marcas.URL(suc.DVR, c.Canal, hd)
				if !mostrar {
					direccion = marcas.Enmascarar(direccion)
				}
				fmt.Printf("  %s: %s\n", marcas.Stream(suc.ID, c.Canal, hd), direccion)
			}
		}
	}
}

func iniciar(ctx context.Context, cfg config.Config) error {
	oyente, err := net.Listen("tcp", cfg.Servidor.Escuchar)
	if err != nil {
		return fmt.Errorf("no se pudo escuchar en %q", cfg.Servidor.Escuchar)
	}
	defer oyente.Close()
	if err := go2rtc.Preparar(cfg); err != nil {
		return err
	}

	ctx, cancelar := context.WithCancel(ctx)
	defer cancelar()
	var avisos sync.WaitGroup
	vigilancia := salud.Nuevo(cfg, salud.Sondas{}, func(c salud.Cambio) {
		mensaje := fmt.Sprintf("🔴 %s sin conexión", c.Nombre)
		if c.EnLinea {
			mensaje = fmt.Sprintf("🟢 %s de vuelta (caída %d min)", c.Nombre, int(c.DuracionCaida.Round(time.Minute)/time.Minute))
		}
		log.Print(mensaje)
		if cfg.Alertas.Telegram.Token != "" {
			avisos.Go(func() {
				if err := alertas.Enviar(ctx, cfg.Alertas.Telegram, mensaje); err != nil && ctx.Err() == nil {
					log.Printf("No se pudo enviar la alerta: %v", err)
				}
			})
		}
	})
	srv := &http.Server{
		Handler:           servidor.Nuevo(cfg, vigilancia.Estado),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       time.Minute,
	}
	var grupo sync.WaitGroup
	grupo.Go(func() { vigilancia.Ejecutar(ctx) })
	grupo.Go(func() { go2rtc.Supervisar(ctx, cfg.Go2rtc) })
	grupo.Go(func() {
		<-ctx.Done()
		limite, fin := context.WithTimeout(context.Background(), 5*time.Second)
		defer fin()
		if err := srv.Shutdown(limite); err != nil {
			_ = srv.Close()
		}
	})
	log.Printf("Monitor escuchando en %s", oyente.Addr())
	err = srv.Serve(oyente)
	cancelar()
	grupo.Wait()
	avisos.Wait()
	if !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("el servidor HTTP se detuvo inesperadamente")
	}
	return nil
}
