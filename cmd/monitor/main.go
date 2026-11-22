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
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/almacen"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/go2rtc"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/marcas"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/recuperacion"
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
	const uso = "Uso: monitor run|check|go2rtc [-config monitor.yaml]\n     monitor instalador [-datos directorio] [-no-abrir]"
	if len(args) == 0 {
		return errors.New(uso)
	}
	switch args[0] {
	case "-h", "--help":
		fmt.Println(uso)
		return nil
	case "instalador":
		return iniciarInstalador(ctx, args[1:])
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
	cfg, err := config.CargarSistema(*ruta)
	if err != nil {
		return err
	}
	a, err := almacen.Abrir(cfg)
	if err != nil {
		return err
	}
	cfg, _ = a.Actual()
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
		return iniciar(ctx, a)
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

func iniciar(ctx context.Context, a *almacen.Almacen) error {
	cambios, desuscribir := a.Suscribir()
	defer desuscribir()
	cfg, version := a.Actual()
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
	avisar := func(mensaje string) {
		log.Print(mensaje)
		actual, _ := a.Actual()
		telegram := actual.Alertas.Telegram
		if telegram.Token == "" {
			return
		}
		avisos.Go(func() {
			if err := alertas.Enviar(ctx, telegram, mensaje); err != nil && ctx.Err() == nil {
				log.Printf("No se pudo enviar la alerta: %v", err)
			}
		})
	}
	vigilancia := salud.Nuevo(cfg, salud.Sondas{}, func(c salud.Cambio) {
		mensaje := fmt.Sprintf("🔴 %s sin conexión", c.Nombre)
		if c.EnLinea {
			mensaje = fmt.Sprintf("🟢 %s de vuelta (caída %d min)", c.Nombre, int(c.DuracionCaida.Round(time.Minute)/time.Minute))
		}
		avisar(mensaje)
	})
	vigilancia.Actualizar(cfg.Sucursales, version)
	reinicios := make(chan struct{}, 1)
	recuperador := recuperacion.Nuevo(a, vigilancia.Estado, avisar)
	var grupo sync.WaitGroup
	grupo.Go(func() { vigilancia.Ejecutar(ctx) })
	grupo.Go(func() { go2rtc.SupervisarConReinicio(ctx, cfg.Go2rtc, reinicios) })
	grupo.Go(func() { recargar(ctx, a, vigilancia, cambios, reinicios, version) })
	grupo.Go(func() { recuperador.Ejecutar(ctx) })
	log.Printf("Monitor escuchando en %s", oyente.Addr())
	err = servirHTTP(ctx, oyente, servidor.NuevaCentral(a, vigilancia.Estado))
	cancelar()
	grupo.Wait()
	avisos.Wait()
	return err
}

func recargar(ctx context.Context, a *almacen.Almacen, vigilancia *salud.Monitor, cambios <-chan struct{}, reinicios chan<- struct{}, version uint64) {
	var reintento <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case _, abierto := <-cambios:
			if !abierto {
				return
			}
		case <-reintento:
			reintento = nil
		}
		cfg, nueva := a.Actual()
		if nueva == version {
			continue
		}
		if err := go2rtc.Preparar(cfg); err != nil {
			log.Printf("No se pudo aplicar la configuración: %v", err)
			reintento = time.After(time.Second)
			continue
		}
		if !cfg.Go2rtc.Externo {
			select {
			case reinicios <- struct{}{}:
			default:
			}
		}
		vigilancia.Actualizar(cfg.Sucursales, nueva)
		version = nueva
	}
}

func servirHTTP(ctx context.Context, oyente net.Listener, handler http.Handler) error {
	ctx, cancelar := context.WithCancel(ctx)
	defer cancelar()
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute}
	terminado := make(chan struct{})
	go func() {
		defer close(terminado)
		<-ctx.Done()
		limite, fin := context.WithTimeout(context.Background(), 5*time.Second)
		defer fin()
		if err := srv.Shutdown(limite); err != nil {
			_ = srv.Close()
		}
	}()
	err := srv.Serve(oyente)
	cancelar()
	<-terminado
	if !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("el servidor HTTP se detuvo inesperadamente")
	}
	return nil
}
