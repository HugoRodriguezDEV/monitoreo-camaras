package servidor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/almacen"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/descubrir"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/dvr"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/web"
)

type destinoCentral struct {
	URL    string `json:"url"`
	Token  string `json:"token"`
	Nombre string `json:"nombre"`
}

type progreso struct {
	Terminado bool                 `json:"terminado"`
	Hechos    int                  `json:"hechos"`
	Total     int                  `json:"total"`
	Hallazgos []descubrir.Hallazgo `json:"hallazgos"`
	Error     string               `json:"error,omitempty"`
}

type trabajo struct {
	progreso
	vence time.Time
}

type instaladorAPI struct {
	ctx          context.Context
	dir          string
	cliente      *http.Client
	nuevoDVR     func(config.DVR) (dvr.Cliente, error)
	buscar       func(context.Context, descubrir.Opciones) ([]descubrir.Hallazgo, error)
	redesLocales func() []netip.Prefix
	mu           sync.Mutex
	central      destinoCentral
	trabajos     map[string]trabajo
}

func NuevoInstalador(ctx context.Context, dir string) (http.Handler, error) {
	return nuevoInstalador(ctx, dir, func(r *http.Request) bool {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		return host == "127.0.0.1" || host == "localhost"
	})
}

// NuevoInstaladorEmbebido admite el host virtual de Wails en Windows.
func NuevoInstaladorEmbebido(ctx context.Context, dir string) (http.Handler, error) {
	return nuevoInstalador(ctx, dir, func(r *http.Request) bool {
		return r.Host == "wails.localhost"
	})
}

func nuevoInstalador(ctx context.Context, dir string, hostLocal func(*http.Request) bool) (http.Handler, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, almacen.ErrGuardar
	}
	i := &instaladorAPI{
		ctx: ctx, dir: dir, nuevoDVR: dvr.Nuevo, buscar: descubrir.Buscar, redesLocales: descubrir.RedesLocales,
		trabajos: make(map[string]trabajo),
		cliente:  &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	if err := almacen.Leer(dir, "central.json", &i.central); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	h := i.handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostLocal(r) {
			fallo(w, 403, "El instalador solo admite acceso local")
			return
		}
		if origen := r.Header.Get("Origin"); origen != "" && origen != "http://"+r.Host {
			fallo(w, 403, "El origen de la solicitud no está permitido")
			return
		}
		h.ServeHTTP(w, r)
	}), nil
}

func (i *instaladorAPI) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/instalador/redes", i.redes)
	mux.HandleFunc("POST /api/instalador/buscar", i.iniciarBusqueda)
	mux.HandleFunc("GET /api/instalador/buscar/{id}", i.progreso)
	mux.HandleFunc("POST /api/instalador/identificar", i.identificar)
	mux.HandleFunc("GET /api/instalador/foto", i.foto)
	mux.HandleFunc("GET /api/instalador/central", i.verCentral)
	mux.HandleFunc("PUT /api/instalador/central", i.guardarCentral)
	mux.HandleFunc("GET /api/instalador/sucursales-central", i.sucursales)
	mux.HandleFunc("POST /api/instalador/enviar", i.enviar)
	mux.HandleFunc("POST /api/instalador/exportar", i.exportar)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fallo(w, 404, "Ruta no encontrada") })
	return servir(config.Config{}, nil, nil, web.Dist, mux, true)
}

func (i *instaladorAPI) redes(w http.ResponseWriter, r *http.Request) {
	redes := []string{}
	for _, red := range i.redesLocales() {
		redes = append(redes, red.String())
	}
	escribirJSON(w, map[string]any{"redes": redes})
}

func (i *instaladorAPI) iniciarBusqueda(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Redes []string `json:"redes"`
	}
	if !leer(w, r, &cuerpo) {
		return
	}
	if len(cuerpo.Redes) > 32 {
		fallo(w, 400, "Selecciona hasta 32 redes")
		return
	}
	opciones := descubrir.Opciones{ONVIF: true}
	for _, texto := range cuerpo.Redes {
		red, err := netip.ParsePrefix(texto)
		if err != nil || !red.Addr().Is4() || red.Bits() < 24 {
			fallo(w, 400, "Indica redes IPv4 de tamaño /24 o menor")
			return
		}
		opciones.Redes = append(opciones.Redes, red.Masked())
	}
	id := secreto()
	i.mu.Lock()
	for clave, t := range i.trabajos {
		if t.Terminado && time.Now().After(t.vence) {
			delete(i.trabajos, clave)
		}
	}
	if len(i.trabajos) >= 64 {
		i.mu.Unlock()
		fallo(w, 429, "Hay demasiadas búsquedas recientes")
		return
	}
	i.trabajos[id] = trabajo{progreso: progreso{Hallazgos: []descubrir.Hallazgo{}}}
	i.mu.Unlock()
	opciones.Progreso = func(hechos, total int) {
		i.mu.Lock()
		defer i.mu.Unlock()
		t := i.trabajos[id]
		t.Hechos, t.Total = hechos, total
		i.trabajos[id] = t
	}
	go func() {
		ctx, fin := context.WithTimeout(i.ctx, 3*time.Minute)
		defer fin()
		hallazgos, err := i.buscar(ctx, opciones)
		i.mu.Lock()
		defer i.mu.Unlock()
		t := i.trabajos[id]
		t.Terminado = true
		t.vence = time.Now().Add(15 * time.Minute)
		t.Hallazgos = append(t.Hallazgos, hallazgos...)
		if err != nil {
			t.Error = "No se pudo completar la búsqueda de DVR"
		} else {
			t.Hechos = t.Total
		}
		i.trabajos[id] = t
	}()
	escribirJSON(w, map[string]string{"id": id})
}

func (i *instaladorAPI) progreso(w http.ResponseWriter, r *http.Request) {
	i.mu.Lock()
	t, ok := i.trabajos[r.PathValue("id")]
	i.mu.Unlock()
	if !ok || (t.Terminado && time.Now().After(t.vence)) {
		fallo(w, 404, "No se encontró la búsqueda")
		return
	}
	escribirJSON(w, t.progreso)
}

func (i *instaladorAPI) clienteDVR(w http.ResponseWriter, d config.DVR) dvr.Cliente {
	if strings.TrimSpace(d.Host) == "" || d.Puerto < 0 || d.Puerto > 65535 || d.PuertoWeb < 0 || d.PuertoWeb > 65535 {
		fallo(w, 400, "Indica el host y un puerto válido del DVR")
		return nil
	}
	switch d.Marca {
	case "hikvision", "dahua", "uniview", "xmeye", "onvif", "rtsp":
	default:
		fallo(w, 400, "Selecciona una marca de DVR válida")
		return nil
	}
	cliente, err := i.nuevoDVR(d)
	if err != nil {
		falloDVR(w, err)
		return nil
	}
	return cliente
}

func (i *instaladorAPI) identificar(w http.ResponseWriter, r *http.Request) {
	var d config.DVR
	if !leer(w, r, &d) {
		return
	}
	cliente := i.clienteDVR(w, d)
	if cliente == nil {
		return
	}
	identidad, err := cliente.Identificar(r.Context())
	if err != nil {
		falloDVR(w, err)
		return
	}
	var lista []sucursalVinculo
	central := i.destino()
	if central.Token != "" {
		_ = i.pedirCentral(r.Context(), central, http.MethodGet, "/api/vinculo/sucursales", nil, &lista)
	}
	escribirJSON(w, map[string]any{"identidad": identidad, "advertencias": advertencias(d, identidad, lista)})
}

func (i *instaladorAPI) foto(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	canal, err := strconv.Atoi(q.Get("canal"))
	if err != nil || canal < 1 {
		fallo(w, 400, "Indica un canal válido")
		return
	}
	puertos := map[string]int{"puerto": 0, "puertoWeb": 0}
	for nombre := range puertos {
		if q.Get(nombre) != "" {
			if puertos[nombre], err = strconv.Atoi(q.Get(nombre)); err != nil {
				fallo(w, 400, "Indica un puerto válido")
				return
			}
		}
	}
	cliente := i.clienteDVR(w, config.DVR{Host: q.Get("host"), Marca: q.Get("marca"), Puerto: puertos["puerto"], PuertoWeb: puertos["puertoWeb"], Usuario: q.Get("usuario"), Password: r.Header.Get("X-DVR-Password")})
	if cliente == nil {
		return
	}
	foto, err := cliente.Foto(r.Context(), canal)
	if err != nil {
		falloDVR(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(foto)
}

func (i *instaladorAPI) destino() destinoCentral {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.central
}

func (i *instaladorAPI) verCentral(w http.ResponseWriter, r *http.Request) {
	c := i.destino()
	escribirJSON(w, map[string]any{"url": c.URL, "vinculado": c.Token != "", "nombre": c.Nombre})
}

type errorCentral struct {
	codigo  int
	mensaje string
}

func (e errorCentral) Error() string { return e.mensaje }

func (i *instaladorAPI) pedirCentral(ctx context.Context, c destinoCentral, metodo, ruta string, cuerpo any, destino any) error {
	if c.URL == "" || c.Token == "" {
		return errorCentral{400, "Primero vincula el instalador con la central"}
	}
	var datos []byte
	if cuerpo != nil {
		var err error
		datos, err = json.Marshal(cuerpo)
		if err != nil {
			return errorCentral{400, "Los datos de la sucursal no son válidos"}
		}
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.URL+ruta, bytes.NewReader(datos))
	if err != nil {
		return errorCentral{400, "La dirección de la central no es válida"}
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := i.cliente.Do(req)
	if err != nil {
		return errorCentral{500, "No se pudo contactar a la central"}
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(destino); err != nil {
			return errorCentral{500, "La central devolvió una respuesta inválida"}
		}
	case 401, 403:
		return errorCentral{resp.StatusCode, "La central rechazó el token de vínculo"}
	case 400, 404, 409:
		return errorCentral{resp.StatusCode, "La central rechazó la solicitud. Revisa los datos y la dirección"}
	default:
		return errorCentral{500, "La central no pudo completar la solicitud"}
	}
	return nil
}

func falloCentral(w http.ResponseWriter, err error) {
	var e errorCentral
	if errors.As(err, &e) {
		fallo(w, e.codigo, e.mensaje)
		return
	}
	fallo(w, 500, "No se pudo comunicar con la central")
}

func (i *instaladorAPI) guardarCentral(w http.ResponseWriter, r *http.Request) {
	var c destinoCentral
	if !leer(w, r, &c) {
		return
	}
	u, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		fallo(w, 400, "Indica la URL de la central, incluida la parte http:// o https://")
		return
	}
	c.URL = strings.TrimSuffix(u.String(), "/")
	c.Token = strings.TrimSpace(c.Token)
	var respuesta struct {
		OK      bool   `json:"ok"`
		Central string `json:"central"`
	}
	if err := i.pedirCentral(r.Context(), c, http.MethodGet, "/api/vinculo/comprobar", nil, &respuesta); err != nil {
		falloCentral(w, err)
		return
	}
	if !respuesta.OK {
		fallo(w, 400, "No se pudo comprobar el vínculo")
		return
	}
	c.Nombre = respuesta.Central
	i.mu.Lock()
	err = almacen.Guardar(i.dir, "central.json", c)
	if err == nil {
		i.central = c
	}
	i.mu.Unlock()
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	escribirJSON(w, map[string]any{"url": c.URL, "vinculado": true, "nombre": c.Nombre})
}

func (i *instaladorAPI) sucursales(w http.ResponseWriter, r *http.Request) {
	lista := []sucursalVinculo{}
	if err := i.pedirCentral(r.Context(), i.destino(), http.MethodGet, "/api/vinculo/sucursales", nil, &lista); err != nil {
		falloCentral(w, err)
		return
	}
	escribirJSON(w, lista)
}

func leerSucursal(w http.ResponseWriter, r *http.Request) (config.Sucursal, bool) {
	var s config.Sucursal
	if !leer(w, r, &s) {
		return s, false
	}
	if err := config.ValidarSucursales([]config.Sucursal{s}); err != nil {
		falloAlmacen(w, err)
		return s, false
	}
	return s, true
}

func (i *instaladorAPI) enviar(w http.ResponseWriter, r *http.Request) {
	s, ok := leerSucursal(w, r)
	if !ok {
		return
	}
	var respuesta struct {
		OK bool `json:"ok"`
	}
	if err := i.pedirCentral(r.Context(), i.destino(), http.MethodPut, "/api/vinculo/sucursales/"+s.ID, s, &respuesta); err != nil {
		falloCentral(w, err)
		return
	}
	if !respuesta.OK {
		fallo(w, 500, "La central no confirmó el envío")
		return
	}
	correcto(w)
}

func (i *instaladorAPI) exportar(w http.ResponseWriter, r *http.Request) {
	s, ok := leerSucursal(w, r)
	if !ok {
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=sucursal-%s.json", s.ID))
	escribirJSON(w, s)
}
