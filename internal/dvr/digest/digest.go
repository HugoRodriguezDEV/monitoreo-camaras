// Package digest implementa la autenticación HTTP Digest de los DVR.
package digest

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

var campo = regexp.MustCompile(`([\w-]+)\s*=\s*(?:"((?:\\.|[^"\\])*)"|([^,\s]+))`)
var escape = strings.NewReplacer(`\`, `\\`, `"`, `\"`)
var desescape = regexp.MustCompile(`\\(.)`)

func Campos(s string) map[string]string {
	if len(s) < 7 || !strings.EqualFold(s[:7], "Digest ") {
		return nil
	}
	m := map[string]string{}
	for _, p := range campo.FindAllStringSubmatch(s[7:], -1) {
		v := p[2]
		if p[3] != "" {
			v = p[3]
		} else {
			v = desescape.ReplaceAllString(v, "$1")
		}
		m[strings.ToLower(p[1])] = v
	}
	return m
}

func Respuesta(m map[string]string, metodo, clave string) string {
	algoritmo := strings.ToUpper(m["algorithm"])
	base := strings.TrimSuffix(algoritmo, "-SESS")
	if base != "" && base != "MD5" && base != "SHA-256" {
		return ""
	}
	hash := func(s string) string {
		if base == "SHA-256" {
			h := sha256.Sum256([]byte(s))
			return hex.EncodeToString(h[:])
		}
		h := md5.Sum([]byte(s))
		return hex.EncodeToString(h[:])
	}
	a := hash(m["username"] + ":" + m["realm"] + ":" + clave)
	if strings.HasSuffix(algoritmo, "-SESS") {
		a = hash(a + ":" + m["nonce"] + ":" + m["cnonce"])
	}
	b := hash(metodo + ":" + m["uri"])
	prefijo := a + ":" + m["nonce"] + ":"
	if m["qop"] == "auth" {
		prefijo += m["nc"] + ":" + m["cnonce"] + ":auth:"
	}
	return hash(prefijo + b)
}

func Autorizacion(reto, metodo, uri, usuario, clave, nonceCliente string) string {
	m := Campos(reto)
	if m["nonce"] == "" {
		return ""
	}
	if m["qop"] != "" {
		admite := false
		for _, q := range strings.Split(m["qop"], ",") {
			admite = admite || strings.TrimSpace(q) == "auth"
		}
		if !admite {
			return ""
		}
		m["qop"] = "auth"
	}
	m["username"], m["uri"], m["cnonce"], m["nc"] = usuario, uri, nonceCliente, "00000001"
	m["response"] = Respuesta(m, metodo, clave)
	if m["response"] == "" {
		return ""
	}
	var partes []string
	for _, k := range []string{"username", "realm", "nonce", "uri", "response", "opaque"} {
		if k != "opaque" || m[k] != "" {
			partes = append(partes, fmt.Sprintf(`%s="%s"`, k, escape.Replace(m[k])))
		}
	}
	if m["algorithm"] != "" {
		partes = append(partes, "algorithm="+m["algorithm"])
	}
	if m["qop"] != "" {
		partes = append(partes, "qop=auth", "nc="+m["nc"])
	}
	if m["qop"] != "" || strings.HasSuffix(strings.ToUpper(m["algorithm"]), "-SESS") {
		partes = append(partes, `cnonce="`+nonceCliente+`"`)
	}
	return "Digest " + strings.Join(partes, ", ")
}
