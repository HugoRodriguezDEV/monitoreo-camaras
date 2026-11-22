package servidor

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

const passwordDVR = "secreto-dvr-de-prueba"
const claveAdmin = "clave-admin-de-prueba"

type clienteFalso struct {
	dvr.Cliente
	identidad dvr.Identidad
	err       error
}

func (d clienteFalso) Identificar(context.Context) (dvr.Identidad, error) { return d.identidad, d.err }
func (d clienteFalso) Reiniciar(context.Context) error                    { return d.err }
func (d clienteFalso) Foto(_ context.Context, canal int) ([]byte, error) {
	if canal != 3 {
		return nil, dvr.ErrNoSoportado
	}
	return []byte{0xff, 0xd8, 0xff, 0xd9}, d.err
}

func centralPrueba(t *testing.T) *central {
	t.Helper()
	cfg, err := config.Leer(strings.NewReader(`
sucursales:
  - id: centro
    nombre: Sucursal Centro
    dvr: {marca: dahua, host: 192.168.2.10, password: secreto-dvr-de-prueba}
    camaras: [{canal: 1, nombre: Entrada}]
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.Datos = t.TempDir()
	a, err := almacen.Abrir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return &central{
		a: a, estado: func() salud.Estado { return salud.Estado{Sucursales: []salud.Sucursal{{ID: "centro", EnLinea: true}}} },
		nuevoDVR: func(config.DVR) (dvr.Cliente, error) {
			return clienteFalso{identidad: dvr.Identidad{Marca: "dahua", Serie: "serie"}, err: nil}, nil
		},
		ahora: time.Now, sesiones: make(map[string]time.Time), intentos: make(map[string]intento),
	}
}

type peticiones struct {
	t         *testing.T
	h         http.Handler
	cookie    *http.Cookie
	token, ip string
}

func (p *peticiones) pedir(metodo, ruta, cuerpo string, codigo int) *httptest.ResponseRecorder {
	p.t.Helper()
	req := httptest.NewRequest(metodo, "http://127.0.0.1"+ruta, strings.NewReader(cuerpo))
	if p.ip != "" {
		req.RemoteAddr = p.ip
	}
	if p.cookie != nil {
		req.AddCookie(p.cookie)
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	w := httptest.NewRecorder()
	p.h.ServeHTTP(w, req)
	if w.Code != codigo {
		p.t.Fatalf("%s %s: HTTP %d, se esperaba %d. %s", metodo, ruta, w.Code, codigo, w.Body.String())
	}
	if (strings.HasPrefix(ruta, "/api/admin/") || strings.HasPrefix(ruta, "/api/vinculo/")) &&
		(strings.Contains(w.Body.String(), passwordDVR) || strings.Contains(w.Body.String(), "passwordHash") || strings.Contains(w.Body.String(), "tokenHash")) {
		p.t.Fatalf("%s %s expuso un secreto", metodo, ruta)
	}
	return w
}

func decodificar[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var dato T
	if err := json.Unmarshal(w.Body.Bytes(), &dato); err != nil {
		t.Fatal(err)
	}
	return dato
}

func TestSesionesYLimiteDeLogin(t *testing.T) {
	c := centralPrueba(t)
	ahora := time.Now()
	c.ahora = func() time.Time { return ahora }
	p := &peticiones{t: t, h: c.handler()}
	const login = `{"password":"` + claveAdmin + `"}`
	const inicio = "/api/admin/inicio"
	w := p.pedir("GET", inicio, "", 200)
	if datos := decodificar[map[string]bool](t, w); datos["configurado"] || datos["sesion"] {
		t.Fatal("hay sesión o contraseña antes de configurar")
	}
	p.pedir("GET", "/api/admin/sucursales", "", 401)
	p.pedir("POST", "/api/admin/configurar", `{"password":"corta"}`, 400)
	w = p.pedir("POST", "/api/admin/configurar", login, 200)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != cookieSesion || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].MaxAge != 43200 {
		t.Fatal("atributos de cookie incorrectos")
	}
	if hash := c.a.Admin().PasswordHash; hash == claveAdmin || !strings.HasPrefix(hash, "$2") {
		t.Fatal("la contraseña no está protegida con bcrypt")
	}
	p.cookie = cookies[0]
	w = p.pedir("GET", inicio, "", 200)
	if datos := decodificar[map[string]bool](t, w); !datos["configurado"] || !datos["sesion"] {
		t.Fatal("no se abrió sesión")
	}
	p.pedir("POST", "/api/admin/configurar", login, 409)
	p.pedir("POST", "/api/admin/logout", "", 200)
	p.pedir("GET", "/api/admin/sucursales", "", 401)
	p.cookie = nil
	for range 4 {
		p.pedir("POST", "/api/admin/login", `{"password":"incorrecta"}`, 401)
	}
	p.pedir("POST", "/api/admin/login", `{"password":"incorrecta"}`, 429)
	ahora = ahora.Add(5*time.Minute - time.Second)
	p.pedir("POST", "/api/admin/login", login, 429)
	otraIP := &peticiones{t: t, h: p.h, ip: "192.0.2.2:12345"}
	otraIP.pedir("POST", "/api/admin/login", login, 200)
	ahora = ahora.Add(time.Second)
	w = p.pedir("POST", "/api/admin/login", login, 200)
	p.cookie = w.Result().Cookies()[0]
	p.pedir("GET", "/api/admin/sucursales", "", 200)
	ahora = ahora.Add(12 * time.Hour)
	p.pedir("GET", "/api/admin/sucursales", "", 401)
}

func TestAdminYVinculoSinCredenciales(t *testing.T) {
	c := centralPrueba(t)
	p := &peticiones{t: t, h: c.handler()}
	w := p.pedir("POST", "/api/admin/configurar", `{"password":"`+claveAdmin+`"}`, 200)
	p.cookie = w.Result().Cookies()[0]
	w = p.pedir("GET", "/api/admin/sucursales", "", 200)
	pruebaVista := func(s map[string]any) {
		t.Helper()
		d, ok := s["dvr"].(map[string]any)
		if !ok || d["password"] != "" || d["tienePassword"] != true {
			t.Fatalf("falta la vista enmascarada: %v", s)
		}
	}
	lista := decodificar[[]map[string]any](t, w)
	pruebaVista(lista[0])
	if lista[0]["enLinea"] != true {
		t.Fatal("falta el estado de salud")
	}
	p.pedir("POST", "/api/admin/sucursales/centro/identificar", "", 200)
	p.pedir("POST", "/api/admin/sucursales/centro/reiniciar", "", 200)
	const editar = `{"id":"centro","nombre":"Centro","dvr":{"marca":"dahua","host":"192.168.2.10","password":""},"camaras":[{"canal":1}]}`
	p.pedir("POST", "/api/admin/sucursales", editar, 409)
	p.pedir("PUT", "/api/admin/sucursales/centro", editar, 200)
	cfg, _ := c.a.Actual()
	if s := cfg.Sucursales[0]; s.DVR.Password != passwordDVR || s.Huella == nil || s.Huella.Serie != "serie" {
		t.Fatal("se perdieron la contraseña o huella")
	}
	p.pedir("PUT", "/api/admin/ajustes", `{"interfaz":{"rotacionSegundos":15},"telegram":{"chatId":"1","token":"token-telegram"}}`, 200)
	w = p.pedir("PUT", "/api/admin/ajustes", `{"interfaz":{"rotacionSegundos":16},"telegram":{"chatId":"2","token":""}}`, 200)
	cfg, _ = c.a.Actual()
	if strings.Contains(w.Body.String(), "token-telegram") || cfg.Alertas.Telegram.Token != "token-telegram" {
		t.Fatal("se expuso o perdió el token de Telegram")
	}
	p.pedir("GET", "/api/vinculo/comprobar", "", 401)
	p.pedir("GET", "/api/vinculo/sucursales", "", 401)
	w = p.pedir("POST", "/api/admin/vinculos", `{"nombre":"Laptop"}`, 200)
	creado := decodificar[map[string]string](t, w)
	bytes, err := base64.RawURLEncoding.DecodeString(creado["token"])
	if len(bytes) != 32 || err != nil || c.a.Admin().Vinculos[0].TokenHash != hashToken(creado["token"]) {
		t.Fatal("el token no cumple el contrato")
	}
	p.token = creado["token"]
	p.pedir("GET", "/api/vinculo/comprobar", "", 200)
	w = p.pedir("PUT", "/api/vinculo/sucursales/norte", `{"id":"norte","nombre":"Norte","dvr":{"marca":"dahua","host":"192.168.4.10","password":"`+passwordDVR+`"},"camaras":[{"canal":1}]}`, 200)
	respuesta := decodificar[struct{ Sucursal map[string]any }](t, w)
	pruebaVista(respuesta.Sucursal)
	w = p.pedir("GET", "/api/vinculo/sucursales", "", 200)
	suc := decodificar[[]map[string]any](t, w)
	if len(suc) != 2 || len(suc[0]["dvr"].(map[string]any)) != 1 {
		t.Fatal("la lista de vínculo expone campos adicionales")
	}
	w = p.pedir("GET", "/api/admin/vinculos", "", 200)
	if strings.Contains(w.Body.String(), p.token) {
		t.Fatal("el token se volvió a mostrar")
	}
	vinculos := decodificar[[]vinculoPublico](t, w)
	if len(vinculos) != 1 || vinculos[0].UltimoUso == nil {
		t.Fatal("falta la fecha de último uso")
	}
	p.pedir("DELETE", "/api/admin/vinculos/"+creado["id"], "", 200)
	p.pedir("GET", "/api/vinculo/comprobar", "", 401)
	p.pedir("PUT", "/api/vinculo/sucursales/norte", editar, 401)
	p.pedir("DELETE", "/api/admin/sucursales/norte", "", 200)
}

func TestRutasPorModo(t *testing.T) {
	c := centralPrueba(t)
	p := &peticiones{t: t, h: c.handler()}
	p.pedir("GET", "/api/instalador/redes", "", 404)
	p.pedir("GET", "/instalar", "", 404)
	h, err := NuevoInstalador(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p.h = h
	p.pedir("GET", "/api/admin/inicio", "", 404)
	p.pedir("GET", "/api/vinculo/comprobar", "", 404)
	p.pedir("GET", "/config.json", "", 404)
	p.pedir("GET", "/api/estado", "", 404)
	p.pedir("GET", "/api/ws", "", 404)
	p.pedir("GET", "/admin", "", 404)
	p.pedir("GET", "/instalar", "", 200)
	p.pedir("GET", "/api/instalador/redes", "", 200)
}

func TestInstaladorConFakes(t *testing.T) {
	c := centralPrueba(t)
	if err := c.a.EditarAdmin(func(a *almacen.Admin) error {
		a.Vinculos = append(a.Vinculos, almacen.Vinculo{ID: "equipo", TokenHash: hashToken("token-de-prueba")})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(c.handler())
	defer srv.Close()
	ctx, fin := context.WithCancel(context.Background())
	defer fin()
	dhcp := true
	identidad := dvr.Identidad{Marca: "dahua", DHCP: &dhcp, Canales: []dvr.Canal{{Canal: 3, CodecSub: "H.265", ResolucionSub: "1280x720", Conectado: false}}}
	i := &instaladorAPI{
		ctx: ctx, dir: t.TempDir(), cliente: srv.Client(), trabajos: make(map[string]trabajo),
		redesLocales: func() []netip.Prefix { return []netip.Prefix{netip.MustParsePrefix("192.168.2.0/24")} },
		buscar: func(_ context.Context, o descubrir.Opciones) ([]descubrir.Hallazgo, error) {
			o.Progreso(1, 2)
			return []descubrir.Hallazgo{{IP: "192.168.2.20", Marca: "dahua"}}, nil
		},
		nuevoDVR: func(d config.DVR) (dvr.Cliente, error) {
			if d.Password != passwordDVR {
				return nil, dvr.ErrCredenciales
			}
			return clienteFalso{identidad: identidad}, nil
		},
	}
	p := &peticiones{t: t, h: i.handler()}
	w := p.pedir("PUT", "/api/instalador/central", `{"url":"`+srv.URL+`","token":"token-de-prueba"}`, 200)
	if strings.Contains(w.Body.String(), "token-de-prueba") {
		t.Fatal("se expuso el token")
	}
	reabierto, err := NuevoInstalador(ctx, i.dir)
	if err != nil {
		t.Fatal(err)
	}
	(&peticiones{t: t, h: reabierto}).pedir("GET", "/api/instalador/sucursales-central", "", 200)
	w = p.pedir("GET", "/api/instalador/redes", "", 200)
	if !strings.Contains(w.Body.String(), "192.168.2.0/24") {
		t.Fatal("no se mostraron las redes")
	}
	w = p.pedir("POST", "/api/instalador/buscar", `{"redes":[]}`, 200)
	id := decodificar[map[string]string](t, w)["id"]
	limite := time.Now().Add(3 * time.Second)
	for {
		w = p.pedir("GET", "/api/instalador/buscar/"+id, "", 200)
		avance := decodificar[progreso](t, w)
		if avance.Terminado {
			if avance.Hechos != 2 || len(avance.Hallazgos) != 1 || avance.Hallazgos[0].IP != "192.168.2.20" {
				t.Fatal("progreso final inválido")
			}
			break
		}
		if time.Now().After(limite) {
			t.Fatal("la búsqueda no terminó")
		}
		time.Sleep(time.Millisecond)
	}
	p.pedir("POST", "/api/instalador/identificar", `{"marca":"dahua","host":"192.168.2.20","password":"incorrecta"}`, 401)
	w = p.pedir("POST", "/api/instalador/identificar", `{"marca":"dahua","host":"192.168.2.20","password":"`+passwordDVR+`"}`, 200)
	avisos := decodificar[struct{ Advertencias []advertencia }](t, w).Advertencias
	codigos := map[string]bool{}
	for _, a := range avisos {
		codigos[a.Codigo] = true
	}
	for _, codigo := range []string{"dhcp", "h265", "resolucion_alta", "canal_sin_senal", "rango_repetido"} {
		if !codigos[codigo] {
			t.Errorf("falta la advertencia %s", codigo)
		}
	}
	otros := advertencias(config.DVR{Marca: "rtsp"}, dvr.Identidad{}, nil)
	if len(otros) != 1 || otros[0].Codigo != "sin_rtsp" {
		t.Fatal("falta advertencia de ruta RTSP")
	}
	req := httptest.NewRequest("GET", "http://127.0.0.1/api/instalador/foto?host=192.168.2.20&marca=dahua&canal=3", nil)
	req.Header.Set("X-DVR-Password", passwordDVR)
	w = httptest.NewRecorder()
	p.h.ServeHTTP(w, req)
	if w.Code != 200 || w.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatal("no se devolvió la foto")
	}
	cuerpo := `{"id":"norte","nombre":"Norte","dvr":{"marca":"dahua","host":"192.168.2.20","password":"` + passwordDVR + `"},"camaras":[{"canal":3}]}`
	p.pedir("POST", "/api/instalador/enviar", cuerpo, 200)
	cfg, _ := c.a.Actual()
	if len(cfg.Sucursales) != 2 || cfg.Sucursales[1].ID != "norte" {
		t.Fatal("no se envió la sucursal a la central")
	}
	w = p.pedir("POST", "/api/instalador/exportar", cuerpo, 200)
	if !strings.Contains(w.Header().Get("Content-Disposition"), "sucursal-norte.json") {
		t.Fatal("falta el nombre de archivo para descargar")
	}
}
