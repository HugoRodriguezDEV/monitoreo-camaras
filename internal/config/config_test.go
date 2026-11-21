package config

import (
	"strings"
	"testing"
)

const sucursalValida = `  - id: centro_1
    dvr:
      marca: hikvision
      host: 192.0.2.1
    camaras:
      - canal: 1
`

func TestLeer(t *testing.T) {
	cfg, err := Leer(strings.NewReader("sucursales:\n" + sucursalValida))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Servidor.Escuchar != ":8080" ||
		cfg.Go2rtc.API != "127.0.0.1:1984" ||
		cfg.Go2rtc.Binario != "/usr/local/bin/go2rtc" ||
		cfg.Go2rtc.Config != "/var/lib/monitor/go2rtc.yaml" ||
		cfg.Go2rtc.Externo ||
		cfg.Interfaz != (Interfaz{25, true, 60}) ||
		cfg.Salud != (Salud{10, 3}) {
		t.Fatal("Los valores predeterminados no coinciden con el contrato")
	}
	cfg, err = Leer(strings.NewReader("interfaz: {rotar_vista_general: false}\ngo2rtc: {externo: true}\nsucursales:\n" + sucursalValida))
	if err != nil || cfg.Interfaz.RotarVistaGeneral || !cfg.Go2rtc.Externo {
		t.Fatalf("No se respetaron los valores booleanos explícitos: %v", err)
	}
}

func TestErroresDeConfiguracion(t *testing.T) {
	base := "sucursales:\n" + sucursalValida
	casos := []struct{ nombre, yaml, errorCampo string }{
		{"Sin sucursales", "sucursales: []\n", "sucursales"},
		{"ID inválido", strings.Replace(base, "centro_1", "Centro-1", 1), "id"},
		{"ID repetido", base + sucursalValida, "centro_1\", id"},
		{"Marca desconocida", strings.Replace(base, "hikvision", "otra", 1), "centro_1\", dvr.marca"},
		{"Ruta RTSP ausente", strings.Replace(base, "hikvision", "rtsp", 1), "centro_1\", dvr.ruta_sub"},
		{"Host ausente", strings.Replace(base, "192.0.2.1", "''", 1), "centro_1\", dvr.host"},
		{"Puerto inválido", strings.Replace(base, "host:", "puerto: 65536\n      host:", 1), "centro_1\", dvr.puerto"},
		{"Sin cámaras", strings.Replace(base, "camaras:\n      - canal: 1", "camaras: []", 1), "centro_1\", camaras"},
		{"Canal cero", strings.Replace(base, "canal: 1", "canal: 0", 1), "centro_1\", camaras.canal"},
		{"Canal repetido", base + "      - canal: 1\n", "centro_1\", camaras.canal: canal 1 repetido"},
		{"API pública", "go2rtc: {api: ':1984'}\n" + base, "go2rtc.api"},
		{"Puerto de API inválido", "go2rtc: {api: '127.0.0.1:0'}\n" + base, "go2rtc.api"},
		{"Intervalo inválido", "salud: {intervalo_segundos: 0}\n" + base, "salud.intervalo_segundos"},
		{"Umbral inválido", "salud: {caido_tras_fallos: 0}\n" + base, "salud.caido_tras_fallos"},
		{"YAML ilegible", "sucursales: [", "YAML"},
		{"Documento adicional", base + "---\n{}\n", "un solo documento"},
	}
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			_, err := Leer(strings.NewReader(caso.yaml))
			if err == nil || !strings.Contains(err.Error(), caso.errorCampo) {
				t.Fatalf("Se esperaba un error con %q; se obtuvo %v", caso.errorCampo, err)
			}
		})
	}
}
