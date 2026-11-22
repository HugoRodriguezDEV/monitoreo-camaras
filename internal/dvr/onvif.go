package dvr

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	dispositivoONVIF = "http://www.onvif.org/ver10/device/wsdl"
	mediosONVIF      = "http://www.onvif.org/ver10/media/wsdl"
	seguridadONVIF   = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
	utilidadONVIF    = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd"
	perfilTokenONVIF = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0"
)

type onvif struct{ equipo }

func escaparXML(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func (o onvif) soap(ctx context.Context, ruta, espacio, accion, argumentos string, v any) error {
	nonce := make([]byte, 20)
	rand.Read(nonce)
	creado := time.Now().UTC().Format(time.RFC3339)
	hash := sha1.New()
	hash.Write(nonce)
	hash.Write([]byte(creado + o.Password))
	cuerpo := fmt.Sprintf(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:w="%s" xmlns:u="%s"><s:Header><w:Security><w:UsernameToken><w:Username>%s</w:Username><w:Password Type="%s#PasswordDigest">%s</w:Password><w:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</w:Nonce><u:Created>%s</u:Created></w:UsernameToken></w:Security></s:Header><s:Body><%s xmlns="%s">%s</%s></s:Body></s:Envelope>`,
		seguridadONVIF, utilidadONVIF, escaparXML(o.Usuario), perfilTokenONVIF, base64.StdEncoding.EncodeToString(hash.Sum(nil)), base64.StdEncoding.EncodeToString(nonce), creado, accion, espacio, argumentos, accion)
	b, errHTTP := o.pedir(ctx, "POST", ruta, `application/soap+xml; charset=utf-8; action="`+espacio+"/"+accion+`"`, []byte(cuerpo))
	if errHTTP != nil && len(b) == 0 {
		return errHTTP
	}
	var sobre struct {
		XMLName xml.Name `xml:"Envelope"`
		Cuerpo  struct {
			Contenido []byte `xml:",innerxml"`
			Fallo     *struct {
				Codigo    string `xml:"Code>Value"`
				Subcodigo string `xml:"Code>Subcode>Value"`
				Motivo    string `xml:"Reason>Text"`
				Codigo11  string `xml:"faultcode"`
			} `xml:"Fault"`
		} `xml:"Body"`
	}
	if err := leerXML(b, &sobre); err != nil {
		if errHTTP != nil {
			return errHTTP
		}
		return err
	}
	if f := sobre.Cuerpo.Fallo; f != nil {
		codigo := strings.ToLower(f.Codigo + " " + f.Subcodigo + " " + f.Codigo11)
		if strings.Contains(codigo, "notauthorized") || strings.Contains(codigo, "failedauthentication") {
			return ErrCredenciales
		}
		return fmt.Errorf("ONVIF rechazó %s: %s", accion, f.Motivo)
	}
	if errHTTP != nil {
		return errHTTP
	}
	var respuesta struct{ XMLName xml.Name }
	if err := leerXML(sobre.Cuerpo.Contenido, &respuesta); err != nil {
		return err
	}
	if respuesta.XMLName.Local != accion+"Response" {
		return fmt.Errorf("respuesta ONVIF inesperada para %s", accion)
	}
	if v != nil {
		return leerXML(sobre.Cuerpo.Contenido, v)
	}
	return nil
}

const rutaDispositivo = "/onvif/device_service"

func (o onvif) dispositivo(ctx context.Context, accion string, v any) error {
	return o.soap(ctx, rutaDispositivo, dispositivoONVIF, accion, "", v)
}

func (o onvif) rutaMedios(ctx context.Context) (string, error) {
	var caps struct {
		Direccion string `xml:"Capabilities>Media>XAddr"`
	}
	err := o.soap(ctx, rutaDispositivo, dispositivoONVIF, "GetCapabilities", "<Category>Media</Category>", &caps)
	if err != nil && !opcional(err) {
		return "", err
	}
	if caps.Direccion == "" {
		return "/onvif/media_service", nil
	}
	return o.rutaAnunciada(caps.Direccion)
}

// Conserva la dirección configurada cuando el DVR anuncia otra IP, por ejemplo detrás de NAT.
func (o onvif) rutaAnunciada(s string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return "", fmt.Errorf("dirección ONVIF inválida: %w", err)
	}
	if u.Path == "" || (u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("dirección ONVIF sin una ruta HTTP válida")
	}
	if u.IsAbs() && strings.EqualFold(u.Hostname(), strings.Trim(o.Host, "[]")) {
		u.User = nil
		return u.String(), nil
	}
	u.Scheme, u.Host, u.User = "", "", nil
	return u.RequestURI(), nil
}

type perfilONVIF struct {
	Token  string `xml:"token,attr"`
	Nombre string `xml:"Name"`
	Fuente string `xml:"VideoSourceConfiguration>SourceToken"`
	Video  *struct {
		Codec string `xml:"Encoding"`
		Ancho int    `xml:"Resolution>Width"`
		Alto  int    `xml:"Resolution>Height"`
		FPS   int    `xml:"RateControl>FrameRateLimit"`
	} `xml:"VideoEncoderConfiguration"`
}

func (o onvif) perfiles(ctx context.Context) (string, []perfilONVIF, error) {
	ruta, err := o.rutaMedios(ctx)
	if err != nil {
		return "", nil, err
	}
	var lista struct {
		Perfiles []perfilONVIF `xml:"Profiles"`
	}
	if err := o.soap(ctx, ruta, mediosONVIF, "GetProfiles", "", &lista); err != nil {
		return "", nil, err
	}
	return ruta, agruparPerfiles(lista.Perfiles), nil
}

func agruparPerfiles(perfiles []perfilONVIF) []perfilONVIF {
	indices := map[string]int{}
	var menores []perfilONVIF
	for _, p := range perfiles {
		fuente := p.Fuente
		if fuente == "" {
			fuente = p.Token
		}
		i, ok := indices[fuente]
		if !ok {
			indices[fuente] = len(menores)
			menores = append(menores, p)
			continue
		}
		if areaPerfil(p) > 0 && (areaPerfil(menores[i]) == 0 || areaPerfil(p) < areaPerfil(menores[i])) {
			p.Nombre = menores[i].Nombre
			menores[i] = p
		}
	}
	return menores
}

func areaPerfil(p perfilONVIF) int64 {
	if p.Video == nil {
		return 0
	}
	return int64(p.Video.Ancho) * int64(p.Video.Alto)
}

func (o onvif) Identificar(ctx context.Context) (Identidad, error) {
	var info struct{ Model, SerialNumber, FirmwareVersion string }
	if err := o.dispositivo(ctx, "GetDeviceInformation", &info); err != nil {
		return Identidad{}, o.errorIdentificar(err)
	}
	id := Identidad{Marca: o.Marca, Modelo: info.Model, Serie: info.SerialNumber, Firmware: info.FirmwareVersion, Canales: []Canal{}}
	var red struct {
		Interfaces []struct {
			Activa    *bool    `xml:"Enabled"`
			MAC       string   `xml:"Info>HwAddress"`
			DHCP      *bool    `xml:"IPv4>Config>DHCP"`
			Manual    []string `xml:"IPv4>Config>Manual>Address"`
			DesdeDHCP string   `xml:"IPv4>Config>FromDHCP>Address"`
		} `xml:"NetworkInterfaces"`
	}
	if err := o.dispositivo(ctx, "GetNetworkInterfaces", &red); err != nil && !opcional(err) {
		return Identidad{}, err
	}
	host := net.ParseIP(strings.Trim(o.Host, "[]"))
	for _, i := range red.Interfaces {
		if i.Activa != nil && !*i.Activa {
			continue
		}
		coincide := false
		for _, ip := range append(i.Manual, i.DesdeDHCP) {
			coincide = coincide || (host != nil && host.Equal(net.ParseIP(ip)))
		}
		if id.MAC == "" || coincide {
			id.MAC, id.DHCP = mac(i.MAC), i.DHCP
		}
		if coincide {
			break
		}
	}
	_, perfiles, err := o.perfiles(ctx)
	if err != nil {
		return Identidad{}, o.errorIdentificar(err)
	}
	for i, p := range perfiles {
		c := Canal{Canal: i + 1, Nombre: nombreCanal(i+1, p.Nombre), Conectado: true}
		if v := p.Video; v != nil {
			c.CodecSub, c.ResolucionSub, c.FpsSub = codec(v.Codec), resolucion(v.Ancho, v.Alto), v.FPS
		}
		id.Canales = append(id.Canales, c)
	}
	return id, nil
}

func (o onvif) errorIdentificar(err error) error {
	if (o.Marca == "xmeye" || o.Marca == "uniview") && !errors.Is(err, ErrCredenciales) && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("da de alta este DVR a mano porque ONVIF no responde: %w (%w)", ErrNoSoportado, err)
	}
	return err
}

func (o onvif) Foto(ctx context.Context, canal int) ([]byte, error) {
	ruta, perfiles, err := o.perfiles(ctx)
	if err != nil {
		return nil, err
	}
	if canal < 1 || canal > len(perfiles) {
		return nil, fmt.Errorf("el canal %d no existe", canal)
	}
	var foto struct {
		URI string `xml:"MediaUri>Uri"`
	}
	err = o.soap(ctx, ruta, mediosONVIF, "GetSnapshotUri", "<ProfileToken>"+escaparXML(perfiles[canal-1].Token)+"</ProfileToken>", &foto)
	if err != nil {
		return nil, err
	}
	destino, err := o.rutaAnunciada(foto.URI)
	if err != nil {
		return nil, err
	}
	return o.pedir(ctx, "GET", destino, "", nil)
}
func (o onvif) Reiniciar(ctx context.Context) error {
	return o.dispositivo(ctx, "SystemReboot", nil)
}
