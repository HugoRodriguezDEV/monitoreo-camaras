package servidor

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/alertas"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/almacen"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/dvr"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/salud"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/web"
	"golang.org/x/crypto/bcrypt"
)

const (
	cookieSesion   = "monitor_sesion"
	duracionSesion = 12 * time.Hour
	bloqueoLogin   = 5 * time.Minute
)

type intento struct {
	fallos int
	vence  time.Time
}

type central struct {
	a               *almacen.Almacen
	estado          func() salud.Estado
	nuevoDVR        func(config.DVR) (dvr.Cliente, error)
	ahora           func() time.Time
	mu              sync.Mutex
	sesiones        map[string]time.Time
	intentos        map[string]intento
	proximaLimpieza time.Time
}

func NuevaCentral(a *almacen.Almacen, estado func() salud.Estado) http.Handler {
	c := &central{a: a, estado: estado, nuevoDVR: dvr.Nuevo, ahora: time.Now, sesiones: make(map[string]time.Time), intentos: make(map[string]intento)}
	return c.handler()
}

func (c *central) handler() http.Handler {
	cfg, _ := c.a.Actual()
	rutas := c.rutas()
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/admin/") && !origenAdminValido(r) {
			fallo(w, 403, "El origen de la solicitud no está permitido")
			return
		}
		rutas.ServeHTTP(w, r)
	})
	return servir(cfg, c.a.Actual, c.estado, web.Dist, api, false)
}

func origenAdminValido(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	origen := r.Header.Get("Origin")
	if origen == "" {
		return true
	}
	u, err := url.Parse(origen)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && strings.EqualFold(u.Host, r.Host)
}

func responder(w http.ResponseWriter, codigo int, dato any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(dato)
}

func fallo(w http.ResponseWriter, codigo int, mensaje string) {
	responder(w, codigo, map[string]string{"error": mensaje})
}

func correcto(w http.ResponseWriter) { escribirJSON(w, map[string]bool{"ok": true}) }

func leer(w http.ResponseWriter, r *http.Request, destino any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	var cuerpo json.RawMessage
	if err := dec.Decode(&cuerpo); err != nil || len(cuerpo) == 0 || cuerpo[0] != '{' ||
		dec.Decode(new(any)) != io.EOF || json.Unmarshal(cuerpo, destino) != nil {
		fallo(w, 400, "El cuerpo debe contener un objeto JSON válido")
		return false
	}
	return true
}

func falloAlmacen(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, almacen.ErrAusente):
		fallo(w, 404, err.Error())
	case errors.Is(err, almacen.ErrExiste), errors.Is(err, almacen.ErrConflicto):
		fallo(w, 409, err.Error())
	case errors.Is(err, almacen.ErrGuardar):
		fallo(w, 500, err.Error())
	default:
		fallo(w, 400, err.Error())
	}
}

func falloDVR(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, dvr.ErrCredenciales):
		fallo(w, 401, "El DVR rechazó el usuario o la contraseña")
	case errors.Is(err, dvr.ErrNoSoportado):
		fallo(w, 400, "La marca no permite esta operación")
	case errors.Is(err, dvr.ErrInalcanzable):
		fallo(w, 400, "El DVR no responde")
	default:
		fallo(w, 500, "No se pudo completar la operación con el DVR")
	}
}

func secreto() string {
	var b [32]byte
	rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (c *central) limpiar(ahora time.Time) {
	if ahora.Before(c.proximaLimpieza) {
		return
	}
	c.proximaLimpieza = ahora.Add(time.Minute)
	for sesion, vence := range c.sesiones {
		if !ahora.Before(vence) {
			delete(c.sesiones, sesion)
		}
	}
	for ip, i := range c.intentos {
		if !ahora.Before(i.vence) {
			delete(c.intentos, ip)
		}
	}
}

func (c *central) tieneSesion(r *http.Request) bool {
	cookie, err := r.Cookie(cookieSesion)
	if err != nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	ahora := c.ahora()
	c.limpiar(ahora)
	return ahora.Before(c.sesiones[hashToken(cookie.Value)])
}

func cookie(w http.ResponseWriter, r *http.Request, valor string, maxEdad int) {
	http.SetCookie(w, &http.Cookie{Name: cookieSesion, Value: valor, Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: maxEdad})
}

func (c *central) abrirSesion(w http.ResponseWriter, r *http.Request) {
	token := secreto()
	c.mu.Lock()
	ahora := c.ahora()
	c.limpiar(ahora)
	c.sesiones[hashToken(token)] = ahora.Add(duracionSesion)
	c.mu.Unlock()
	cookie(w, r, token, int(duracionSesion.Seconds()))
	correcto(w)
}

func (c *central) inicio(w http.ResponseWriter, r *http.Request) {
	escribirJSON(w, map[string]bool{"configurado": c.a.Admin().PasswordHash != "", "sesion": c.tieneSesion(r)})
}

func (c *central) configurar(w http.ResponseWriter, r *http.Request) {
	if c.a.Admin().PasswordHash != "" {
		fallo(w, 409, "La contraseña ya está configurada")
		return
	}
	var cuerpo struct {
		Password string `json:"password"`
	}
	if !leer(w, r, &cuerpo) {
		return
	}
	if utf8.RuneCountInString(cuerpo.Password) < 8 || len(cuerpo.Password) > 72 {
		fallo(w, 400, "La contraseña debe tener al menos 8 caracteres y ocupar hasta 72 bytes")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(cuerpo.Password), bcrypt.DefaultCost)
	if err != nil {
		fallo(w, 500, "No se pudo proteger la contraseña")
		return
	}
	err = c.a.EditarAdmin(func(admin *almacen.Admin) error {
		if admin.PasswordHash != "" {
			return almacen.ErrExiste
		}
		admin.PasswordHash = string(hash)
		return nil
	})
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	c.abrirSesion(w, r)
}

func (c *central) validarLogin(ip, password string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	ahora := c.ahora()
	c.limpiar(ahora)
	i := c.intentos[ip]
	if !ahora.Before(i.vence) {
		i = intento{}
	}
	if i.fallos >= 5 {
		return 429
	}
	hash := c.a.Admin().PasswordHash
	if hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil {
		delete(c.intentos, ip)
		return 200
	}
	i.fallos++
	i.vence = c.ahora().Add(bloqueoLogin)
	c.intentos[ip] = i
	if i.fallos >= 5 {
		return 429
	}
	return 401
}

func (c *central) login(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Password string `json:"password"`
	}
	if !leer(w, r, &cuerpo) {
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	switch c.validarLogin(ip, cuerpo.Password) {
	case 200:
		c.abrirSesion(w, r)
	case 429:
		fallo(w, 429, "Demasiados intentos. Vuelve a intentarlo en 5 minutos")
	default:
		fallo(w, 401, "Contraseña incorrecta")
	}
}

func (c *central) logout(w http.ResponseWriter, r *http.Request) {
	if token, err := r.Cookie(cookieSesion); err == nil {
		c.mu.Lock()
		delete(c.sesiones, hashToken(token.Value))
		c.mu.Unlock()
	}
	cookie(w, r, "", -1)
	correcto(w)
}

type sucursalAdmin struct {
	config.Sucursal
	DVR struct {
		config.DVR
		TienePassword bool `json:"tienePassword"`
	} `json:"dvr"`
	EnLinea bool   `json:"enLinea"`
	Error   string `json:"error"`
}

func vistaSucursal(s config.Sucursal, estado salud.Estado) sucursalAdmin {
	vista := sucursalAdmin{Sucursal: s}
	vista.DVR.DVR = s.DVR
	vista.DVR.TienePassword = s.DVR.Password != ""
	vista.DVR.Password = ""
	vista.Sucursal.DVR.Password = ""
	for _, actual := range estado.Sucursales {
		if s.ID == actual.ID {
			vista.EnLinea, vista.Error = actual.EnLinea, actual.Error
			break
		}
	}
	return vista
}

func (c *central) sucursales(w http.ResponseWriter, r *http.Request) {
	cfg, _ := c.a.Actual()
	estado := c.estado()
	lista := make([]sucursalAdmin, 0, len(cfg.Sucursales))
	for _, s := range cfg.Sucursales {
		lista = append(lista, vistaSucursal(s, estado))
	}
	escribirJSON(w, lista)
}

func (c *central) guardarSucursal(w http.ResponseWriter, r *http.Request, crear, vinculo bool) {
	var s config.Sucursal
	if !leer(w, r, &s) {
		return
	}
	if id := r.PathValue("id"); id != "" {
		if s.ID != "" && s.ID != id {
			fallo(w, 400, "El id no coincide con la ruta")
			return
		}
		s.ID = id
	}
	err := c.a.EditarSucursales(func(lista []config.Sucursal) ([]config.Sucursal, error) {
		i := almacen.Indice(lista, s.ID)
		switch {
		case i < 0 && (crear || vinculo):
			return append(lista, s), nil
		case i < 0:
			return nil, almacen.ErrAusente
		case crear:
			return nil, almacen.ErrExiste
		}
		anterior := lista[i]
		if s.DVR.Password == "" {
			s.DVR.Password = anterior.DVR.Password
		}
		if s.Huella == nil && anterior.DVR.Host == s.DVR.Host && anterior.DVR.Marca == s.DVR.Marca {
			s.Huella = anterior.Huella
		}
		lista[i] = s
		return lista, nil
	})
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	if vinculo {
		escribirJSON(w, map[string]any{"ok": true, "sucursal": vistaSucursal(s, c.estado())})
		return
	}
	correcto(w)
}

func (c *central) borrarSucursal(w http.ResponseWriter, r *http.Request) {
	err := c.a.EditarSucursales(func(lista []config.Sucursal) ([]config.Sucursal, error) {
		i := almacen.Indice(lista, r.PathValue("id"))
		if i < 0 {
			return nil, almacen.ErrAusente
		}
		return slices.Delete(lista, i, i+1), nil
	})
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	correcto(w)
}

func (c *central) operarDVR(w http.ResponseWriter, r *http.Request, reiniciar bool) {
	cfg, _ := c.a.Actual()
	i := almacen.Indice(cfg.Sucursales, r.PathValue("id"))
	if i < 0 {
		fallo(w, 404, "No se encontró la sucursal")
		return
	}
	s := cfg.Sucursales[i]
	fallar := func(err error) {
		if reiniciar && errors.Is(err, dvr.ErrNoSoportado) {
			fallo(w, 501, "La marca no permite reiniciar el DVR")
			return
		}
		falloDVR(w, err)
	}
	cliente, err := c.nuevoDVR(s.DVR)
	if err != nil {
		fallar(err)
		return
	}
	if reiniciar {
		if err := cliente.Reiniciar(r.Context()); err != nil {
			fallar(err)
			return
		}
		correcto(w)
		return
	}
	identidad, err := cliente.Identificar(r.Context())
	if err != nil {
		falloDVR(w, err)
		return
	}
	err = c.a.EditarSucursales(func(lista []config.Sucursal) ([]config.Sucursal, error) {
		i := almacen.Indice(lista, s.ID)
		if i < 0 || lista[i].DVR != s.DVR {
			return nil, almacen.ErrConflicto
		}
		lista[i].Huella = &config.Huella{MAC: identidad.MAC, Serie: identidad.Serie, Modelo: identidad.Modelo, Marca: identidad.Marca, Actualizada: c.ahora().UTC()}
		return lista, nil
	})
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	escribirJSON(w, map[string]any{"identidad": identidad, "advertencias": advertencias(s.DVR, identidad, nil)})
}

type vistaAjustes struct {
	Interfaz config.Interfaz `json:"interfaz"`
	Telegram struct {
		ChatID     string `json:"chatId"`
		TieneToken bool   `json:"tieneToken"`
	} `json:"telegram"`
}

func (c *central) ajustes(w http.ResponseWriter, r *http.Request) {
	cfg, _ := c.a.Actual()
	vista := vistaAjustes{Interfaz: cfg.Interfaz}
	vista.Telegram.ChatID = cfg.Alertas.Telegram.ChatID
	vista.Telegram.TieneToken = cfg.Alertas.Telegram.Token != ""
	escribirJSON(w, vista)
}

func (c *central) guardarAjustes(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Interfaz config.Interfaz `json:"interfaz"`
		Telegram config.Telegram `json:"telegram"`
	}
	if !leer(w, r, &cuerpo) {
		return
	}
	if cuerpo.Interfaz.RotacionSegundos < 0 || cuerpo.Interfaz.VolverACuadriculaSegundos < 0 {
		fallo(w, 400, "Los tiempos de la interfaz deben ser positivos o cero")
		return
	}
	err := c.a.EditarAjustes(func(a *almacen.Ajustes) {
		a.Interfaz = cuerpo.Interfaz
		a.Alertas.Telegram.ChatID = cuerpo.Telegram.ChatID
		if cuerpo.Telegram.Token != "" {
			a.Alertas.Telegram.Token = cuerpo.Telegram.Token
		}
	})
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	c.ajustes(w, r)
}

func (c *central) pruebaTelegram(w http.ResponseWriter, r *http.Request) {
	cfg, _ := c.a.Actual()
	if cfg.Alertas.Telegram.Token == "" || cfg.Alertas.Telegram.ChatID == "" {
		fallo(w, 400, "Configura el token y el chat de Telegram primero")
		return
	}
	if err := alertas.Enviar(r.Context(), cfg.Alertas.Telegram, "✅ Prueba del monitor de cámaras"); err != nil {
		fallo(w, 500, err.Error())
		return
	}
	correcto(w)
}

type vinculoPublico struct {
	ID        string     `json:"id"`
	Nombre    string     `json:"nombre"`
	Creado    time.Time  `json:"creado"`
	UltimoUso *time.Time `json:"ultimoUso"`
}

func (c *central) vinculos(w http.ResponseWriter, r *http.Request) {
	lista := []vinculoPublico{}
	for _, v := range c.a.Admin().Vinculos {
		lista = append(lista, vinculoPublico{v.ID, v.Nombre, v.Creado, v.UltimoUso})
	}
	escribirJSON(w, lista)
}

func (c *central) crearVinculo(w http.ResponseWriter, r *http.Request) {
	var cuerpo struct {
		Nombre string `json:"nombre"`
	}
	if !leer(w, r, &cuerpo) {
		return
	}
	nombre := strings.TrimSpace(cuerpo.Nombre)
	if nombre == "" {
		fallo(w, 400, "Escribe un nombre para el vínculo")
		return
	}
	token, id := secreto(), secreto()
	err := c.a.EditarAdmin(func(a *almacen.Admin) error {
		a.Vinculos = append(a.Vinculos, almacen.Vinculo{ID: id, Nombre: nombre, TokenHash: hashToken(token), Creado: c.ahora().UTC()})
		return nil
	})
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	escribirJSON(w, map[string]string{"id": id, "nombre": nombre, "token": token})
}

func (c *central) revocarVinculo(w http.ResponseWriter, r *http.Request) {
	err := c.a.EditarAdmin(func(a *almacen.Admin) error {
		i := slices.IndexFunc(a.Vinculos, func(v almacen.Vinculo) bool { return v.ID == r.PathValue("id") })
		if i < 0 {
			return almacen.ErrAusente
		}
		a.Vinculos = slices.Delete(a.Vinculos, i, i+1)
		return nil
	})
	if err != nil {
		falloAlmacen(w, err)
		return
	}
	correcto(w)
}

func (c *central) autorizarVinculo(w http.ResponseWriter, r *http.Request) bool {
	esquema, token, _ := strings.Cut(r.Header.Get("Authorization"), " ")
	if !strings.EqualFold(esquema, "Bearer") || token == "" {
		fallo(w, 401, "Se requiere un token de vínculo")
		return false
	}
	hash := hashToken(token)
	err := c.a.EditarAdmin(func(a *almacen.Admin) error {
		for i, v := range a.Vinculos {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(v.TokenHash)) == 1 {
				ahora := c.ahora().UTC()
				a.Vinculos[i].UltimoUso = &ahora
				return nil
			}
		}
		return almacen.ErrAusente
	})
	if errors.Is(err, almacen.ErrAusente) {
		fallo(w, 401, "El token de vínculo no es válido")
		return false
	}
	if err != nil {
		falloAlmacen(w, err)
		return false
	}
	return true
}

type sucursalVinculo struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
	DVR    struct {
		Host string `json:"host"`
	} `json:"dvr"`
}

func (c *central) sucursalesVinculo(w http.ResponseWriter, r *http.Request) {
	cfg, _ := c.a.Actual()
	lista := make([]sucursalVinculo, 0, len(cfg.Sucursales))
	for _, s := range cfg.Sucursales {
		vista := sucursalVinculo{ID: s.ID, Nombre: s.Nombre}
		vista.DVR.Host = s.DVR.Host
		lista = append(lista, vista)
	}
	escribirJSON(w, lista)
}

func (c *central) rutas() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/admin/inicio", c.inicio)
	mux.HandleFunc("POST /api/admin/configurar", c.configurar)
	mux.HandleFunc("POST /api/admin/login", c.login)
	admin := func(patron string, h http.HandlerFunc) {
		mux.HandleFunc(patron, func(w http.ResponseWriter, r *http.Request) {
			if !c.tieneSesion(r) {
				fallo(w, 401, "Inicia sesión para continuar")
				return
			}
			h(w, r)
		})
	}
	admin("POST /api/admin/logout", c.logout)
	admin("GET /api/admin/sucursales", c.sucursales)
	admin("POST /api/admin/sucursales", func(w http.ResponseWriter, r *http.Request) { c.guardarSucursal(w, r, true, false) })
	admin("PUT /api/admin/sucursales/{id}", func(w http.ResponseWriter, r *http.Request) { c.guardarSucursal(w, r, false, false) })
	admin("DELETE /api/admin/sucursales/{id}", c.borrarSucursal)
	admin("POST /api/admin/sucursales/{id}/identificar", func(w http.ResponseWriter, r *http.Request) { c.operarDVR(w, r, false) })
	admin("POST /api/admin/sucursales/{id}/reiniciar", func(w http.ResponseWriter, r *http.Request) { c.operarDVR(w, r, true) })
	admin("GET /api/admin/ajustes", c.ajustes)
	admin("PUT /api/admin/ajustes", c.guardarAjustes)
	admin("POST /api/admin/telegram/prueba", c.pruebaTelegram)
	admin("GET /api/admin/vinculos", c.vinculos)
	admin("POST /api/admin/vinculos", c.crearVinculo)
	admin("DELETE /api/admin/vinculos/{id}", c.revocarVinculo)
	vinculo := func(patron string, h http.HandlerFunc) {
		mux.HandleFunc(patron, func(w http.ResponseWriter, r *http.Request) {
			if c.autorizarVinculo(w, r) {
				h(w, r)
			}
		})
	}
	vinculo("GET /api/vinculo/comprobar", func(w http.ResponseWriter, r *http.Request) {
		nombre, err := os.Hostname()
		if err != nil || nombre == "" {
			nombre = "Monitor central"
		}
		escribirJSON(w, map[string]any{"ok": true, "central": nombre})
	})
	vinculo("GET /api/vinculo/sucursales", c.sucursalesVinculo)
	vinculo("PUT /api/vinculo/sucursales/{id}", func(w http.ResponseWriter, r *http.Request) { c.guardarSucursal(w, r, false, true) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { fallo(w, 404, "Ruta no encontrada") })
	return mux
}
