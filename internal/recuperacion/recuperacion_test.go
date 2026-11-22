package recuperacion

import (
	"context"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/almacen"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/descubrir"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/dvr"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/salud"
)

type dvrFalso struct {
	dvr.Cliente
	identidad dvr.Identidad
}

func (d dvrFalso) Identificar(context.Context) (dvr.Identidad, error) { return d.identidad, nil }

func TestRecuperarPorHuella(t *testing.T) {
	for _, caso := range []struct {
		nombre    string
		identidad dvr.Identidad
		coincide  bool
	}{
		{"MAC", dvr.Identidad{MAC: "AA-BB-CC-DD-EE-FF", Serie: "distinta"}, true},
		{"Serie", dvr.Identidad{Serie: "SERIE-123", MAC: "ff:ee:dd:cc:bb:aa"}, true},
		{"Sin coincidencia", dvr.Identidad{MAC: "11:22:33:44:55:66", Serie: "ajena"}, false},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			cfg, err := config.Leer(strings.NewReader(`
sucursales:
  - id: centro
    nombre: Sucursal Centro
    dvr: {marca: dahua, host: 192.168.2.64, usuario: admin, password: prueba}
    camaras: [{canal: 1}]
    huella: {mac: "aa:bb:cc:dd:ee:ff", serie: SERIE-123}
`))
			if err != nil {
				t.Fatal(err)
			}
			cfg.Datos = t.TempDir()
			a, err := almacen.Abrir(cfg)
			if err != nil {
				t.Fatal(err)
			}
			ahora := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
			caida := ahora.Add(-2 * time.Minute)
			aviso := ""
			r := Nuevo(a, func() salud.Estado {
				_, v := a.Actual()
				return salud.Estado{ConfigVersion: v, Sucursales: []salud.Sucursal{{ID: "centro", Desde: caida}}}
			}, func(s string) { aviso = s })
			r.ahora = func() time.Time { return ahora }
			barridos := 0
			r.buscar = func(_ context.Context, o descubrir.Opciones) ([]descubrir.Hallazgo, error) {
				barridos++
				if o.ONVIF || len(o.Redes) != 1 || o.Redes[0].String() != "192.168.2.0/24" {
					t.Fatalf("opciones inesperadas: %+v", o)
				}
				return []descubrir.Hallazgo{
					{IP: "192.168.3.1", Marca: "dahua", Puertos: []int{80}},
					{IP: "192.168.2.90", Marca: "hikvision", Puertos: []int{8000}},
					{IP: "192.168.2.112", Marca: "dahua", Puertos: []int{37777}},
				}, nil
			}
			r.nuevoDVR = func(d config.DVR) (dvr.Cliente, error) {
				if d.Host != "192.168.2.112" || d.Password != "prueba" || d.Usuario != "admin" {
					t.Fatal("se consultó un DVR fuera de los candidatos esperados")
				}
				return dvrFalso{identidad: caso.identidad}, nil
			}
			lanzar := func(s config.Sucursal, red netip.Prefix) {
				r.recuperar(context.Background(), s, red)
				i := r.intentos[s.ID]
				i.activo = false
				r.intentos[s.ID] = i
			}
			r.revisar(lanzar)
			if barridos != 0 {
				t.Fatal("se buscó antes del umbral")
			}
			ahora = ahora.Add(time.Minute)
			r.revisar(lanzar)
			actual, v := a.Actual()
			if barridos != 1 {
				t.Fatal("no se hizo el primer barrido")
			}
			if caso.coincide {
				if actual.Sucursales[0].DVR.Host != "192.168.2.112" || v != 2 || !strings.Contains(aviso, "192.168.2.64 a 192.168.2.112") {
					t.Fatal("no se guardó o avisó el cambio de host")
				}
			} else if actual.Sucursales[0].DVR.Host != "192.168.2.64" || v != 1 || aviso != "" {
				t.Fatal("se cambió la sucursal sin coincidencia")
			}
			ahora = ahora.Add(10*time.Minute - time.Second)
			r.revisar(lanzar)
			if barridos != 1 {
				t.Fatal("se repitió antes de 10 minutos")
			}
			ahora = ahora.Add(time.Second)
			r.revisar(lanzar)
			if barridos != 2 {
				t.Fatal("no se volvió a buscar después de 10 minutos")
			}
		})
	}
}
