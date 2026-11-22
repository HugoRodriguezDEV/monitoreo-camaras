package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/servidor"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	ventanas "github.com/wailsapp/wails/v2/pkg/options/windows"
	"golang.org/x/sys/windows"
)

const titulo = "Monitor de cámaras — Instalador"

func main() {
	if err := ejecutar(); err != nil {
		mensaje, _ := windows.UTF16PtrFromString("No se pudo abrir el asistente. " + err.Error())
		cabecera, _ := windows.UTF16PtrFromString(titulo)
		_, _ = windows.MessageBox(0, mensaje, cabecera, windows.MB_OK|windows.MB_ICONERROR)
		os.Exit(1)
	}
}

func ejecutar() error {
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()

	datos := config.DatosUsuario()
	handler, err := servidor.NuevoInstaladorEmbebido(ctx, datos)
	if err != nil {
		return err
	}

	return wails.Run(&options.App{
		Title:     titulo,
		Width:     1100,
		Height:    800,
		MinWidth:  380,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == "/" {
					// Wails v2 abre la raíz de su host virtual en Windows.
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = io.WriteString(w, `<!doctype html><html lang="es"><meta charset="utf-8"><meta http-equiv="refresh" content="0; url=/instalar"><title>Asistente de instalación</title><a href="/instalar">Abrir el asistente</a></html>`)
					return
				}
				handler.ServeHTTP(w, r)
			}),
		},
		Windows: &ventanas.Options{
			WebviewUserDataPath: filepath.Join(datos, "WebView2"),
			Messages: &ventanas.Messages{
				InstallationRequired: "El asistente necesita Microsoft Edge WebView2. Pulsa Aceptar para descargarlo e instalarlo. Espera a que termine la instalación.",
				UpdateRequired:       "El asistente necesita actualizar Microsoft Edge WebView2. Pulsa Aceptar para descargar e instalar la actualización.",
				MissingRequirements:  "Microsoft Edge WebView2",
				Webview2NotInstalled: "Microsoft Edge WebView2 no está instalado.",
				Error:                "Error de Microsoft Edge WebView2",
				FailedToInstall:      "No se pudo instalar WebView2. Vuelve a intentarlo.",
				WebView2ProcessCrash: "WebView2 se cerró inesperadamente. Vuelve a abrir el asistente.",
			},
		},
	})
}
