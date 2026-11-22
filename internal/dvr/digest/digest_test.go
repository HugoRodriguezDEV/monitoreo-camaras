package digest

import (
	"strings"
	"testing"
)

func TestDigestRFC2617(t *testing.T) {
	reto := `Digest realm="testrealm@host.com", qop="auth,auth-int", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", opaque="5ccc069c403ebaf9f0171e9517f40e41"`
	cabecera := Autorizacion(reto, "GET", "/dir/index.html", "Mufasa", "Circle Of Life", "0a4f113b")
	m := Campos(cabecera)
	if m["response"] != "6629fae49393a05397450978507c4ef1" || m["qop"] != "auth" || m["nc"] != "00000001" {
		t.Fatalf("autorización inesperada: %s", cabecera)
	}
}

func TestDigestClasico(t *testing.T) {
	reto := `Digest realm="DVR", nonce="c0c0"`
	cabecera := Autorizacion(reto, "PUT", "/ISAPI/System/reboot", `admin"\`, "clave", "abcdef")
	m := Campos(cabecera)
	if m["username"] != `admin"\` || m["response"] == "" || strings.Contains(cabecera, "qop=") {
		t.Fatalf("autorización inesperada: %s", cabecera)
	}
}
