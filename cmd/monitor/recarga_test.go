package main

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/almacen"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/go2rtc"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/salud"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/servidor"
)

func TestRecargaEnCaliente(t *testing.T) {
	cfg, err := config.Leer(strings.NewReader(`
sucursales:
  - id: centro
    dvr: {marca: hikvision, host: 192.0.2.10}
    camaras: [{canal: 1}]
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Datos = t.TempDir()
	cfg.Go2rtc.Config = filepath.Join(cfg.Datos, "go2rtc.yaml")
	a, err := almacen.Abrir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := go2rtc.Preparar(cfg); err != nil {
		t.Fatal(err)
	}
	verificarYAML := func(host string) {
		t.Helper()
		b, err := os.ReadFile(cfg.Go2rtc.Config)
		if err != nil || !strings.Contains(string(b), host) {
			t.Fatalf("no se generó el YAML esperado para %s: %v", host, err)
		}
	}
	verificarYAML("192.0.2.10")
	cambios, desuscribir := a.Suscribir()
	defer desuscribir()
	_, version := a.Actual()
	vigilancia := salud.Nuevo(cfg, salud.Sondas{}, nil)
	vigilancia.Actualizar(cfg.Sucursales, version)
	reinicios := make(chan struct{}, 1)
	ctx, fin := context.WithCancel(context.Background())
	listo := make(chan struct{})
	go func() { defer close(listo); recargar(ctx, a, vigilancia, cambios, reinicios, version) }()
	defer func() { fin(); <-listo }()
	h := servidor.NuevaCentral(a, vigilancia.Estado)
	err = a.EditarSucursales(func(lista []config.Sucursal) ([]config.Sucursal, error) {
		lista[0].DVR.Host = "192.0.2.20"
		lista[0].Camaras = append(lista[0].Camaras, config.Camara{Canal: 2})
		return lista, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-reinicios:
	case <-time.After(5 * time.Second):
		t.Fatal("no se solicitó reinicio")
	}
	verificarYAML("192.0.2.20")
	limite := time.Now().Add(5 * time.Second)
	for {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/api/estado", nil))
		var estado salud.Estado
		if err := json.Unmarshal(w.Body.Bytes(), &estado); err != nil {
			t.Fatal(err)
		}
		if estado.ConfigVersion == version+1 {
			break
		}
		if time.Now().After(limite) {
			t.Fatal("no subió configVersion")
		}
		time.Sleep(10 * time.Millisecond)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/config.json", nil))
	if !strings.Contains(w.Body.String(), "centro_c2") {
		t.Fatal("la configuración pública quedó desactualizada")
	}
}
