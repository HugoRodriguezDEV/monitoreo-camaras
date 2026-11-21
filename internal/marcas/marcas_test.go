package marcas

import (
	"net/url"
	"strings"
	"testing"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

func TestURLPorMarca(t *testing.T) {
	casos := []struct{ marca, protocolo, puerto, sub, hd string }{
		{"hikvision", "rtsp", "554", "/Streaming/Channels/302", "/Streaming/Channels/301"},
		{"dahua", "rtsp", "554", "/cam/realmonitor?channel=3&subtype=1", "/cam/realmonitor?channel=3&subtype=0"},
		{"uniview", "rtsp", "554", "/unicast/c3/s1/live", "/unicast/c3/s0/live"},
		{"xmeye", "dvrip", "34567", "?channel=2&subtype=1", "?channel=2&subtype=0"},
		{"onvif", "onvif", "80", "?subtype=1", "?subtype=0"},
		{"rtsp", "rtsp", "554", "/live/ch3/sub", "/live/ch3/main"},
	}
	for _, c := range casos {
		t.Run(c.marca, func(t *testing.T) {
			d := config.DVR{Marca: c.marca, Host: "192.0.2.1", Usuario: "us@rio", Password: "p@ss:w/rd#", RutaSub: "/live/ch{canal}/sub", RutaPrincipal: "/live/ch{canal}/main"}
			for hd, ruta := range map[bool]string{false: c.sub, true: c.hd} {
				generado := URL(d, 3, hd)
				esperado := c.protocolo + "://us%40rio:p%40ss%3Aw%2Frd%23@192.0.2.1:" + c.puerto + ruta
				if generado != esperado {
					t.Fatalf("URL %q; se esperaba %q", generado, esperado)
				}
				u, err := url.Parse(generado)
				if err != nil {
					t.Fatal(err)
				}
				if clave, _ := u.User.Password(); clave != d.Password {
					t.Fatalf("Contraseña decodificada incorrecta")
				}
			}
		})
	}
}

func TestRTSPSinRutaPrincipal(t *testing.T) {
	d := config.DVR{Marca: "rtsp", Host: "::1", Puerto: 8554, RutaSub: "/ch{canal}"}
	if got := URL(d, 2, true); got != "rtsp://[::1]:8554/ch2" {
		t.Fatalf("URL incorrecta: %s", got)
	}
	if Stream("s1", 2, true) != "s1_c2_hd" || Stream("s1", 2, false) != "s1_c2" {
		t.Fatal("Nombre de stream incorrecto")
	}
}

func TestEnmascarar(t *testing.T) {
	entrada := "dvrip://us%40rio:p%40ss%3Aw%2Frd%23@casa:34567?channel=0"
	esperado := "dvrip://us%40rio:****@casa:34567?channel=0"
	if got := Enmascarar(entrada); got != esperado || strings.Contains(got, "p%40ss") {
		t.Fatalf("Enmascarado incorrecto: %s", got)
	}
	for _, sinClave := range []string{"rtsp://casa:554/live", "rtsp://usuario@casa:554/live"} {
		if Enmascarar(sinClave) != sinClave {
			t.Fatalf("Cambió una URL sin contraseña: %s", sinClave)
		}
	}
}
