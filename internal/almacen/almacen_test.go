package almacen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

func semilla(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Leer(strings.NewReader(`
interfaz: {rotacion_segundos: 42}
alertas: {telegram: {token: secreto, chat_id: "1"}}
sucursales:
  - id: centro
    dvr: {marca: hikvision, host: 192.0.2.10, password: clave-dvr}
    camaras: [{canal: 1, nombre: Entrada}]
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Datos = t.TempDir()
	return cfg
}

func TestMigracionYPermisos(t *testing.T) {
	cfg := semilla(t)
	a, err := Abrir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	actual, v := a.Actual()
	if actual.Sucursales[0].DVR.Password != "clave-dvr" || actual.Interfaz.RotacionSegundos != 42 || actual.Alertas.Telegram.Token != "secreto" || v != 1 {
		t.Fatal("no se migraron todos los datos")
	}
	for _, nombre := range []string{"sucursales.json", "ajustes.json", "admin.json"} {
		info, err := os.Stat(filepath.Join(cfg.Datos, nombre))
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("%s tiene permisos %o", nombre, info.Mode().Perm())
		}
	}
	actual.Sucursales[0].Camaras[0].Nombre = "Cambio fuera del almacén"
	cfg.Sucursales[0].DVR.Host = ""
	cfg.Interfaz.RotacionSegundos = 12
	cfg.Alertas.Telegram.Token = "otro"
	a, err = Abrir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ = a.Actual()
	if actual.Sucursales[0].DVR.Host != "192.0.2.10" || actual.Sucursales[0].Camaras[0].Nombre != "Entrada" ||
		actual.Interfaz.RotacionSegundos != 42 || actual.Alertas.Telegram.Token != "secreto" {
		t.Fatal("el YAML reemplazó el JSON migrado")
	}
}

func TestFalloDeEscrituraNoPublica(t *testing.T) {
	cfg := semilla(t)
	a, err := Abrir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cambios, fin := a.Suscribir()
	defer fin()
	ruta := filepath.Join(cfg.Datos, "sucursales.json")
	if err := os.Rename(ruta, ruta+".original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(ruta, 0700); err != nil {
		t.Fatal(err)
	}
	err = a.EditarSucursales(func(lista []config.Sucursal) ([]config.Sucursal, error) {
		lista[0].Camaras[0].Nombre = "Cambio"
		return lista, nil
	})
	if !errors.Is(err, ErrGuardar) {
		t.Fatalf("se esperaba fallo de escritura: %v", err)
	}
	actual, v := a.Actual()
	if v != 1 || actual.Sucursales[0].Camaras[0].Nombre != "Entrada" {
		t.Fatal("se publicó una escritura fallida")
	}
	select {
	case <-cambios:
		t.Fatal("se notificó una escritura fallida")
	default:
	}
	restos, err := filepath.Glob(filepath.Join(cfg.Datos, ".monitor-*"))
	if err != nil || len(restos) != 0 {
		t.Fatalf("quedaron temporales: %v, %v", restos, err)
	}
}

func TestEdicionesConcurrentesYLecturaAtomica(t *testing.T) {
	cfg := semilla(t)
	a, err := Abrir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	cambios, fin := a.Suscribir()
	defer fin()
	const n = 24
	var grupo sync.WaitGroup
	for i := range n {
		grupo.Go(func() {
			err := a.EditarSucursales(func(lista []config.Sucursal) ([]config.Sucursal, error) {
				s := lista[0]
				s.ID = fmt.Sprintf("s%d", i)
				return append(lista, s), nil
			})
			if err != nil {
				t.Error(err)
			}
		})
	}
	grupo.Go(func() {
		for range n * 10 {
			var lista []config.Sucursal
			if err := Leer(cfg.Datos, "sucursales.json", &lista); err != nil || len(lista) < 1 {
				t.Errorf("se vio un archivo incompleto: %v", err)
				return
			}
		}
	})
	grupo.Wait()
	actual, v := a.Actual()
	if v != 1+n || len(actual.Sucursales) != 1+n {
		t.Fatalf("se perdieron ediciones: versión %d, %d sucursales", v, len(actual.Sucursales))
	}
	select {
	case <-cambios:
	default:
		t.Fatal("no llegó notificación")
	}
}
