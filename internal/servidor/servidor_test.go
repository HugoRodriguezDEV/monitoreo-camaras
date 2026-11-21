package servidor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/salud"
)

func TestConfigSinCredenciales(t *testing.T) {
	cfg := config.Config{
		Salud:   config.Salud{IntervaloSegundos: 10, CaidoTrasFallos: 3},
		Alertas: config.Alertas{Telegram: config.Telegram{Token: "token-secreto", ChatID: "chat-secreto"}},
		Sucursales: []config.Sucursal{{
			ID: "s1", Nombre: "Sucursal Centro",
			DVR:     config.DVR{Usuario: "usuario-secreto", Password: "p@ss:w/rd#", Host: "host-interno"},
			Camaras: []config.Camara{{Canal: 1, Nombre: "Entrada", Clave: true}},
		}},
	}
	respuesta := httptest.NewRecorder()
	Nuevo(cfg, nil).ServeHTTP(respuesta, httptest.NewRequest(http.MethodGet, "/config.json", nil))
	if respuesta.Code != 200 {
		t.Fatalf("HTTP %d", respuesta.Code)
	}
	for _, privada := range []string{"token-secreto", "chat-secreto", "usuario-secreto", "p@ss:w/rd#", "p%40ss", "host-interno", "password"} {
		if strings.Contains(respuesta.Body.String(), privada) {
			t.Fatal("La respuesta incluye datos privados")
		}
	}
	var publica interfaz
	if err := json.Unmarshal(respuesta.Body.Bytes(), &publica); err != nil {
		t.Fatal(err)
	}
	if publica.SinConexionSegundos != 30 || publica.Go2rtc != "" || len(publica.Sucursales) != 1 ||
		len(publica.Sucursales[0].Camaras) != 1 || publica.Sucursales[0].Camaras[0] != (camara{"s1_c1", "s1_c1_hd", "Entrada", true}) {
		t.Fatalf("Configuración pública incorrecta: %+v", publica)
	}
}

func TestRutasDelProxy(t *testing.T) {
	falso := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.RequestURI())
	}))
	defer falso.Close()
	cfg := configProxy(falso.URL)
	h := Nuevo(cfg, func() salud.Estado { return salud.Estado{} })
	casos := []struct {
		ruta   string
		codigo int
		cuerpo string
	}{
		{"/api/streams", 404, "Ruta no encontrada\n"},
		{"/api/ws?src=s1_c1", 200, "/api/ws?src=s1_c1"},
		{"/api/ws?src=no_existe", 404, "Stream no encontrado\n"},
		{"/api/ws?src=s1_c1&dst=s1_c1", 404, "Stream no encontrado\n"},
		{"/video-stream.js", 200, "/video-stream.js"},
		{"/video-rtc.js", 200, "/video-rtc.js"},
		{"/api/ws/../streams", 404, "Ruta no encontrada\n"},
	}
	for _, c := range casos {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.ruta, nil))
		if w.Code != c.codigo || w.Body.String() != c.cuerpo {
			t.Errorf("%s: HTTP %d, cuerpo %q", c.ruta, w.Code, w.Body.String())
		}
	}
}

func configProxy(destino string) config.Config {
	return config.Config{
		Go2rtc:     config.Go2rtc{API: strings.TrimPrefix(destino, "http://")},
		Sucursales: []config.Sucursal{{ID: "s1", Camaras: []config.Camara{{Canal: 1}}}},
	}
}

func TestProxyWebSocket(t *testing.T) {
	const trama = "\x81\x04hola"
	const aceptacion = "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	remoto := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/api/ws?src=s1_c1_hd" || r.Header.Get("Upgrade") != "websocket" ||
			r.Header.Get("Origin") != "http://"+r.Host {
			http.Error(w, "Petición WebSocket inválida", http.StatusBadRequest)
			return
		}
		conn, buffer, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error("No se pudo tomar la conexión WebSocket")
			return
		}
		defer conn.Close()
		fmt.Fprintf(buffer, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: %s\r\n\r\n%s", aceptacion, trama)
		if err := buffer.Flush(); err != nil {
			t.Error("No se pudo escribir la trama WebSocket")
		}
	}))
	defer remoto.Close()
	monitor := httptest.NewServer(Nuevo(configProxy(remoto.URL), nil))
	defer monitor.Close()
	req, err := http.NewRequest(http.MethodGet, monitor.URL+"/api/ws?src=s1_c1_hd", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Origin", monitor.URL)
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	cliente := http.Client{Timeout: 3 * time.Second}
	resp, err := cliente.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols || resp.Header.Get("Sec-WebSocket-Accept") != aceptacion {
		t.Fatalf("Actualización de WebSocket incorrecta: HTTP %d", resp.StatusCode)
	}
	datos := make([]byte, len(trama))
	if _, err := io.ReadFull(resp.Body, datos); err != nil || string(datos) != trama {
		t.Fatalf("Trama incorrecta: %q, %v", datos, err)
	}
}

func TestInterfaz(t *testing.T) {
	dist := fstest.MapFS{"index.html": {Data: []byte("<h1>Monitor</h1>")}, "assets/app.js": {Data: []byte("alert('Monitor')")}}
	for _, c := range []struct {
		ruta, cuerpo string
		archivos     fstest.MapFS
	}{
		{"/", "<h1>Monitor</h1>", dist},
		{"/sucursales/s1", "<h1>Monitor</h1>", dist},
		{"/assets/app.js", "alert('Monitor')", dist},
		{"/", "Interfaz no compilada", fstest.MapFS{}},
	} {
		w := httptest.NewRecorder()
		nuevo(config.Config{}, nil, c.archivos).ServeHTTP(w, httptest.NewRequest(http.MethodGet, c.ruta, nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), c.cuerpo) {
			t.Errorf("%s: HTTP %d, cuerpo %q", c.ruta, w.Code, w.Body.String())
		}
	}
}
