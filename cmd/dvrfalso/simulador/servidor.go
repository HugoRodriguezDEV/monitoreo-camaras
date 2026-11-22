// Package simulador sirve los protocolos de los DVR para las pruebas de desarrollo.
package simulador

import (
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strconv"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/dvr/digest"
)

type Opciones struct {
	Marca, Usuario, Password, MAC, Serie string
	Canales                              int
	H265                                 map[int]bool
	DHCP                                 bool
}
type servidor struct {
	Opciones
	nonce string
}

func Nuevo(op Opciones) http.Handler {
	return servidor{op, rand.Text()}
}

func (s servidor) digest(w http.ResponseWriter, r *http.Request) bool {
	campos := digest.Campos(r.Header.Get("Authorization"))
	valida := campos["username"] == s.Usuario && campos["realm"] == s.Marca && campos["nonce"] == s.nonce &&
		campos["uri"] == r.URL.RequestURI() && campos["qop"] == "auth" && campos["nc"] != "" &&
		campos["cnonce"] != "" && campos["algorithm"] == "MD5" && campos["response"] != ""
	if valida && subtle.ConstantTimeCompare([]byte(campos["response"]), []byte(digest.Respuesta(campos, r.Method, s.Password))) == 1 {
		return true
	}
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm="%s", nonce="%s", algorithm=MD5, qop="auth"`, s.Marca, s.nonce))
	http.Error(w, "usuario o contraseña incorrectos", http.StatusUnauthorized)
	return false
}

func (s servidor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Server", s.Marca)
	if s.Marca == "onvif" && (r.URL.Path == "/onvif/device_service" || r.URL.Path == "/onvif/media_service") {
		s.onvif(w, r)
		return
	}
	if !s.digest(w, r) {
		return
	}
	switch s.Marca {
	case "hikvision":
		s.hikvision(w, r)
	case "dahua":
		s.dahua(w, r)
	case "onvif":
		if r.Method == "GET" && r.URL.Path == "/foto" {
			s.foto(w, r.URL.Query().Get("canal"))
			return
		}
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s servidor) codec(canal int) string {
	if s.H265[canal] {
		return "H265"
	}
	return "H264"
}

func (s servidor) foto(w http.ResponseWriter, valor string) {
	n, err := strconv.Atoi(valor)
	if err != nil || n < 1 || n > s.Canales {
		http.Error(w, "el canal no existe", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	guardarFoto(w, n)
}
