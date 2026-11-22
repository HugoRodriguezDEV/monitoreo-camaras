package simulador

import (
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s servidor) onvif(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return
	}
	var solicitud struct {
		Token struct {
			Usuario  string `xml:"Username"`
			Nonce    string
			Created  string
			Password struct {
				Tipo  string `xml:"Type,attr"`
				Valor string `xml:",chardata"`
			}
		} `xml:"Header>Security>UsernameToken"`
		Cuerpo struct {
			Accion struct {
				XMLName xml.Name
				Perfil  string `xml:"ProfileToken"`
			} `xml:",any"`
		} `xml:"Body"`
	}
	dec := xml.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(&solicitud); err != nil {
		http.Error(w, "solicitud SOAP inválida", 400)
		return
	}
	t := solicitud.Token
	nonce, err := base64.StdEncoding.DecodeString(t.Nonce)
	creado, errHora := time.Parse(time.RFC3339, t.Created)
	hash := sha1.New()
	hash.Write(nonce)
	hash.Write([]byte(t.Created + s.Password))
	clave := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	if err != nil || len(nonce) == 0 || errHora != nil || time.Since(creado) > 5*time.Minute || time.Until(creado) > time.Minute ||
		!strings.HasSuffix(t.Password.Tipo, "#PasswordDigest") || t.Usuario != s.Usuario ||
		subtle.ConstantTimeCompare([]byte(t.Password.Valor), []byte(clave)) != 1 {
		http.Error(w, "usuario o contraseña incorrectos", 401)
		return
	}
	accion := solicitud.Cuerpo.Accion.XMLName.Local
	medio := accion == "GetProfiles" || accion == "GetSnapshotUri"
	ruta, espacio := "/onvif/device_service", "http://www.onvif.org/ver10/device/wsdl"
	if medio {
		ruta, espacio = "/onvif/media_service", "http://www.onvif.org/ver10/media/wsdl"
	}
	if ruta != r.URL.Path || espacio != solicitud.Cuerpo.Accion.XMLName.Space {
		http.NotFound(w, r)
		return
	}
	var b strings.Builder
	switch accion {
	case "GetDeviceInformation":
		fmt.Fprintf(&b, "<Manufacturer>Simulador</Manufacturer><Model>ONVIF-SIMULADO</Model><FirmwareVersion>4.0.0</FirmwareVersion><SerialNumber>%s</SerialNumber><HardwareId>1</HardwareId>", escapar(s.Serie))
	case "GetNetworkInterfaces":
		fmt.Fprintf(&b, "<NetworkInterfaces token=\"eth0\"><tt:Enabled>true</tt:Enabled><tt:Info><tt:HwAddress>%s</tt:HwAddress></tt:Info><tt:IPv4><tt:Enabled>true</tt:Enabled><tt:Config><tt:DHCP>%t</tt:DHCP></tt:Config></tt:IPv4></NetworkInterfaces>", escapar(s.MAC), s.DHCP)
	case "GetCapabilities":
		fmt.Fprintf(&b, "<Capabilities><tt:Media><tt:XAddr>http://%s/onvif/media_service</tt:XAddr></tt:Media></Capabilities>", escapar(r.Host))
	case "GetProfiles":
		for n := 1; n <= s.Canales; n++ {
			perfil(&b, n, "principal", "H264", 1920, 1080)
			perfil(&b, n, "sub", s.codec(n), 640, 360)
		}
	case "GetSnapshotUri":
		token := solicitud.Cuerpo.Accion.Perfil
		clase, numero, ok := strings.Cut(token, "-")
		n, err := strconv.Atoi(numero)
		if !ok || err != nil || n < 1 || n > s.Canales || (clase != "principal" && clase != "sub") {
			http.Error(w, "perfil desconocido", 400)
			return
		}
		fmt.Fprintf(&b, "<MediaUri><tt:Uri>http://%s/foto?canal=%d</tt:Uri></MediaUri>", escapar(r.Host), n)
	case "SystemReboot":
		fmt.Fprint(&b, "<Message>Reinicio simulado</Message>")
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/soap+xml")
	fmt.Fprintf(w, `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema"><s:Body><%sResponse xmlns="%s">%s</%sResponse></s:Body></s:Envelope>`, accion, espacio, b.String(), accion)
}

func perfil(b *strings.Builder, n int, tipo, codec string, ancho, alto int) {
	fmt.Fprintf(b, `<Profiles token="%s-%d"><tt:Name>Canal %d</tt:Name><tt:VideoSourceConfiguration><tt:SourceToken>fuente-%d</tt:SourceToken></tt:VideoSourceConfiguration><tt:VideoEncoderConfiguration><tt:Encoding>%s</tt:Encoding><tt:Resolution><tt:Width>%d</tt:Width><tt:Height>%d</tt:Height></tt:Resolution><tt:RateControl><tt:FrameRateLimit>15</tt:FrameRateLimit></tt:RateControl></tt:VideoEncoderConfiguration></Profiles>`, tipo, n, n, n, codec, ancho, alto)
}
