package servidor

import (
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/marcas"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/salud"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/web"
)

type camara struct {
	Stream   string `json:"stream"`
	StreamHD string `json:"streamHd"`
	Nombre   string `json:"nombre"`
	Clave    bool   `json:"clave"`
}

type sucursal struct {
	ID      string   `json:"id"`
	Nombre  string   `json:"nombre"`
	Camaras []camara `json:"camaras"`
}

type interfaz struct {
	Go2rtc                    string     `json:"go2rtc"`
	RotacionSegundos          int        `json:"rotacionSegundos"`
	RotarVistaGeneral         bool       `json:"rotarVistaGeneral"`
	VolverACuadriculaSegundos int        `json:"volverACuadriculaSegundos"`
	SinConexionSegundos       int        `json:"sinConexionSegundos"`
	Sucursales                []sucursal `json:"sucursales"`
}

func configuracionPublica(cfg config.Config) interfaz {
	publica := interfaz{
		RotacionSegundos:          cfg.Interfaz.RotacionSegundos,
		RotarVistaGeneral:         cfg.Interfaz.RotarVistaGeneral,
		VolverACuadriculaSegundos: cfg.Interfaz.VolverACuadriculaSegundos,
		SinConexionSegundos:       cfg.Salud.IntervaloSegundos * cfg.Salud.CaidoTrasFallos,
		Sucursales:                make([]sucursal, 0, len(cfg.Sucursales)),
	}
	for _, s := range cfg.Sucursales {
		vista := sucursal{ID: s.ID, Nombre: s.Nombre, Camaras: make([]camara, 0, len(s.Camaras))}
		for _, c := range s.Camaras {
			vista.Camaras = append(vista.Camaras, camara{
				Stream: marcas.Stream(s.ID, c.Canal, false), StreamHD: marcas.Stream(s.ID, c.Canal, true),
				Nombre: c.Nombre, Clave: c.Clave,
			})
		}
		publica.Sucursales = append(publica.Sucursales, vista)
	}
	return publica
}

func escribirJSON(w http.ResponseWriter, valor any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(valor)
}

func Nuevo(cfg config.Config, estado func() salud.Estado) http.Handler {
	return nuevo(cfg, estado, web.Dist)
}

func nuevo(cfg config.Config, estado func() salud.Estado, dist fs.FS) http.Handler {
	destino := &url.URL{Scheme: "http", Host: cfg.Go2rtc.API}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.RawPath = ""
			r.SetURL(destino)
			// go2rtc compara el origen del WebSocket con el host de la petición.
			r.Out.Host = r.In.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "go2rtc no está disponible", http.StatusBadGateway)
		},
	}
	publica := configuracionPublica(cfg)
	streams := make(map[string]bool)
	for _, s := range publica.Sucursales {
		for _, c := range s.Camaras {
			streams[c.Stream], streams[c.StreamHD] = true, true
		}
	}
	archivos := http.FileServerFS(dist)
	_, err := fs.Stat(dist, "index.html")
	compilada := err == nil

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ws", "/video-stream.js", "/video-rtc.js":
			if r.Method == http.MethodGet {
				if r.URL.Path == "/api/ws" && r.URL.RawQuery != "" {
					params, err := url.ParseQuery(r.URL.RawQuery)
					// Solo se ofrecen streams del monitor. go2rtc también acepta fuentes dinámicas.
					if err != nil || len(params) != 1 || len(params["src"]) != 1 || !streams[params.Get("src")] {
						http.Error(w, "Stream no encontrado", http.StatusNotFound)
						return
					}
				}
				proxy.ServeHTTP(w, r)
				return
			}
		case "/config.json", "/api/estado", "/salud":
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				switch r.URL.Path {
				case "/config.json":
					escribirJSON(w, publica)
				case "/api/estado":
					escribirJSON(w, estado())
				case "/salud":
					w.Header().Set("Content-Type", "text/plain; charset=utf-8")
					_, _ = io.WriteString(w, "ok\n")
				}
				return
			}
		default:
			limpia := path.Clean(r.URL.Path)
			if limpia == "/api" || strings.HasPrefix(limpia, "/api/") || strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "Ruta no encontrada", http.StatusNotFound)
				return
			}
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				nombre := strings.TrimPrefix(limpia, "/")
				if info, err := fs.Stat(dist, nombre); err == nil && !info.IsDir() {
					if nombre == "index.html" {
						nombre = ""
					}
					copia := r.Clone(r.Context())
					copia.URL.Path, copia.URL.RawPath = "/"+nombre, ""
					archivos.ServeHTTP(w, copia)
					return
				}
				if compilada {
					copia := r.Clone(r.Context())
					copia.URL.Path, copia.URL.RawPath = "/", ""
					archivos.ServeHTTP(w, copia)
					return
				}
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = io.WriteString(w, "<!doctype html><html lang=\"es\"><meta charset=\"utf-8\"><title>Interfaz no compilada</title><h1>Interfaz no compilada</h1><p>Corre <code>npm run build</code> en web/.</p></html>")
				return
			}
		}
		w.Header().Set("Allow", "GET, HEAD")
		if r.URL.Path == "/api/ws" || r.URL.Path == "/video-stream.js" || r.URL.Path == "/video-rtc.js" {
			w.Header().Set("Allow", "GET")
		}
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
	})
}
