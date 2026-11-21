package marcas

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

func Stream(sucursal string, canal int, hd bool) string {
	nombre := fmt.Sprintf("%s_c%d", sucursal, canal)
	if hd {
		nombre += "_hd"
	}
	return nombre
}

func URL(d config.DVR, canal int, hd bool) string {
	protocolo := "rtsp"
	subtipo := 1
	if hd {
		subtipo = 0
	}
	var ruta string
	switch d.Marca {
	case "hikvision":
		ruta = fmt.Sprintf("/Streaming/Channels/%d0%d", canal, subtipo+1)
	case "dahua":
		ruta = fmt.Sprintf("/cam/realmonitor?channel=%d&subtype=%d", canal, subtipo)
	case "uniview":
		ruta = fmt.Sprintf("/unicast/c%d/s%d/live", canal, subtipo)
	case "xmeye":
		protocolo = "dvrip"
		ruta = fmt.Sprintf("?channel=%d&subtype=%d", canal-1, subtipo)
	case "onvif":
		protocolo = "onvif"
		ruta = fmt.Sprintf("?subtype=%d", subtipo)
	case "rtsp":
		ruta = d.RutaSub
		if hd && d.RutaPrincipal != "" {
			ruta = d.RutaPrincipal
		}
		ruta = strings.ReplaceAll(ruta, "{canal}", strconv.Itoa(canal))
	}
	base := url.URL{Scheme: protocolo, Host: d.Direccion()}
	if d.Usuario != "" || d.Password != "" {
		base.User = url.UserPassword(d.Usuario, d.Password)
	}
	return base.String() + ruta
}

// Enmascarar oculta la contraseña incluso si la ruta de la cámara no se puede analizar.
func Enmascarar(valor string) string {
	inicio := strings.Index(valor, "://")
	if inicio < 0 {
		return valor
	}
	inicio += 3
	resto := valor[inicio:]
	fin := strings.IndexAny(resto, "/?#")
	if fin < 0 {
		fin = len(resto)
	}
	autoridad := resto[:fin]
	arroba := strings.LastIndexByte(autoridad, '@')
	if arroba < 0 {
		return valor
	}
	separador := strings.IndexByte(autoridad[:arroba], ':')
	if separador < 0 {
		return valor
	}
	return valor[:inicio+separador+1] + "****" + resto[arroba:]
}
