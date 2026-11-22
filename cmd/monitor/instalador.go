package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os/exec"
	"runtime"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/servidor"
)

func iniciarInstalador(ctx context.Context, args []string) error {
	const uso = "Uso: monitor instalador [-datos directorio] [-no-abrir]"
	opciones := flag.NewFlagSet("instalador", flag.ContinueOnError)
	opciones.SetOutput(io.Discard)
	datos := opciones.String("datos", config.DatosUsuario(), "Directorio de datos del usuario")
	noAbrir := opciones.Bool("no-abrir", false, "No abrir el navegador")
	if err := opciones.Parse(args); errors.Is(err, flag.ErrHelp) {
		fmt.Println(uso)
		return nil
	} else if err != nil || opciones.NArg() != 0 || *datos == "" {
		return errors.New(uso)
	}
	h, err := servidor.NuevoInstalador(ctx, *datos)
	if err != nil {
		return err
	}
	oyente, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("no se pudo iniciar el servidor local del instalador")
	}
	defer oyente.Close()
	direccion := "http://" + oyente.Addr().String() + "/instalar"
	log.Printf("Instalador disponible en %s", direccion)
	if !*noAbrir {
		go func() {
			if err := abrirNavegador(ctx, direccion); err != nil && ctx.Err() == nil {
				log.Printf("No se pudo abrir el navegador. Abre %s", direccion)
			}
		}()
	}
	return servirHTTP(ctx, oyente, h)
}

func abrirNavegador(ctx context.Context, direccion string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", direccion).Run()
	case "darwin":
		return exec.CommandContext(ctx, "open", direccion).Run()
	default:
		return exec.CommandContext(ctx, "xdg-open", direccion).Run()
	}
}
