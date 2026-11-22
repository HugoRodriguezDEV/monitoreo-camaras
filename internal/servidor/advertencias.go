package servidor

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/dvr"
)

type advertencia struct {
	Nivel   string `json:"nivel"`
	Codigo  string `json:"codigo"`
	Mensaje string `json:"mensaje"`
}

func menuVideo(marca string) string {
	switch marca {
	case "hikvision":
		return "En Configuración → Vídeo/Audio, elige el flujo secundario."
	case "dahua":
		return "En Cámara → Codificación, elige el flujo extra."
	case "xmeye":
		return "En Sistema → Codificación, elige el flujo secundario."
	case "uniview":
		return "En Cámara → Vídeo, elige el flujo secundario."
	default:
		return "En los ajustes de vídeo del DVR, elige el flujo secundario."
	}
}

func advertencias(d config.DVR, identidad dvr.Identidad, lista []sucursalVinculo) []advertencia {
	avisos := []advertencia{}
	agregar := func(codigo, mensaje string) { avisos = append(avisos, advertencia{"aviso", codigo, mensaje}) }
	if identidad.DHCP != nil && *identidad.DHCP {
		agregar("dhcp", "El DVR toma su IP del módem: fíjala. En Red → TCP/IP, desactiva la asignación automática y guarda la IP, máscara y puerta de enlace.")
	}
	if d.Marca == "rtsp" && strings.TrimSpace(d.RutaSub) == "" {
		agregar("sin_rtsp", "Falta la ruta de vídeo del flujo secundario. Consulta la ruta RTSP en el manual del DVR.")
	}
	menu := menuVideo(d.Marca)
	for _, c := range identidad.Canales {
		codec := strings.NewReplacer(".", "", "-", "", " ", "").Replace(strings.ToLower(c.CodecSub))
		if codec == "h265" || codec == "h265+" || codec == "hevc" {
			agregar("h265", fmt.Sprintf("La cámara %d manda H.265; cámbiala a H.264 en el DVR. %s", c.Canal, menu))
		}
		if !c.Conectado {
			agregar("canal_sin_senal", fmt.Sprintf("La cámara %d no tiene señal. Revisa su alimentación y cable.", c.Canal))
		}
		ancho, alto, _ := strings.Cut(strings.ToLower(c.ResolucionSub), "x")
		a, _ := strconv.Atoi(strings.TrimSpace(ancho))
		b, _ := strconv.Atoi(strings.TrimSpace(alto))
		if a > 704 || b > 576 {
			agregar("resolucion_alta", fmt.Sprintf("El flujo secundario de la cámara %d supera 704×576. %s Baja la resolución a 704×576 o menos.", c.Canal, menu))
		}
	}
	host, err := netip.ParseAddr(d.Host)
	if err == nil && host.Is4() {
		red := netip.PrefixFrom(host, 24).Masked()
		octetos := red.Addr().As4()
		for _, s := range lista {
			otro, err := netip.ParseAddr(s.DVR.Host)
			if err == nil && red.Contains(otro) {
				agregar("rango_repetido", fmt.Sprintf("La %s ya usa %d.%d.%d.x", s.Nombre, octetos[0], octetos[1], octetos[2]))
			}
		}
	}
	return avisos
}
