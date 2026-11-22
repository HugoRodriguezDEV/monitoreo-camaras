package dvr

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type hikvision struct{ equipo }

type dispositivoHik struct {
	XMLName  struct{} `xml:"DeviceInfo"`
	Modelo   string   `xml:"model"`
	Serie    string   `xml:"serialNumber"`
	MAC      string   `xml:"macAddress"`
	Firmware string   `xml:"firmwareVersion"`
}
type entradaHik struct {
	ID      int    `xml:"id"`
	Nombre  string `xml:"name"`
	EnLinea *bool  `xml:"online"`
}
type flujoHik struct {
	Video struct {
		Codec string `xml:"videoCodecType"`
		Ancho int    `xml:"videoResolutionWidth"`
		Alto  int    `xml:"videoResolutionHeight"`
		FPS   int    `xml:"maxFrameRate"`
	} `xml:"Video"`
}

func (h hikvision) leer(ctx context.Context, ruta string, v any) error {
	b, err := h.pedir(ctx, "GET", "/ISAPI/"+ruta, "", nil)
	if err != nil {
		return err
	}
	return leerXML(b, v)
}

func (h hikvision) Identificar(ctx context.Context) (Identidad, error) {
	var info dispositivoHik
	if err := h.leer(ctx, "System/deviceInfo", &info); err != nil {
		return Identidad{}, err
	}
	id := Identidad{Marca: h.Marca, Modelo: info.Modelo, Serie: info.Serie, MAC: mac(info.MAC), Firmware: info.Firmware, Canales: []Canal{}}
	var red struct {
		Interfaces []struct {
			MAC  string `xml:"Link>MACAddress"`
			Tipo string `xml:"IPAddress>addressingType"`
		} `xml:"NetworkInterface"`
	}
	if err := h.leer(ctx, "System/Network/interfaces", &red); err != nil && !opcional(err) {
		return Identidad{}, err
	}
	for _, i := range red.Interfaces {
		if id.MAC == "" {
			id.MAC = mac(i.MAC)
		}
		var dhcp bool
		switch strings.ToLower(strings.TrimSpace(i.Tipo)) {
		case "dhcp":
			dhcp = true
		case "static":
		default:
			continue
		}
		id.DHCP = &dhcp
		if mac(i.MAC) == id.MAC {
			break
		}
	}
	var lista struct {
		Proxy []entradaHik `xml:"InputProxyChannel"`
		Video []entradaHik `xml:"VideoInputChannel"`
	}
	err := h.leer(ctx, "ContentMgmt/InputProxy/channels", &lista)
	if err != nil && !opcional(err) {
		return Identidad{}, err
	}
	entradas := lista.Proxy
	if len(entradas) == 0 {
		err = h.leer(ctx, "System/Video/inputs/channels", &lista)
		if err != nil && !opcional(err) {
			return Identidad{}, err
		}
		entradas = lista.Video
	}
	for _, e := range entradas {
		if e.ID < 1 {
			continue
		}
		c := Canal{Canal: e.ID, Nombre: nombreCanal(e.ID, e.Nombre), Conectado: e.EnLinea == nil || *e.EnLinea}
		var flujo flujoHik
		err = h.leer(ctx, fmt.Sprintf("Streaming/channels/%d02", e.ID), &flujo)
		if err != nil && !opcional(err) {
			return Identidad{}, err
		}
		v := flujo.Video
		c.CodecSub, c.ResolucionSub, c.FpsSub = codec(v.Codec), resolucion(v.Ancho, v.Alto), v.FPS/100
		id.Canales = append(id.Canales, c)
	}
	return id, nil
}

func opcional(err error) bool {
	return !errors.Is(err, ErrCredenciales) && !errors.Is(err, ErrInalcanzable) && !errors.Is(err, context.Canceled)
}

func (h hikvision) Foto(ctx context.Context, canal int) ([]byte, error) {
	return h.pedir(ctx, "GET", fmt.Sprintf("/ISAPI/Streaming/channels/%d01/picture", canal), "", nil)
}
func (h hikvision) Reiniciar(ctx context.Context) error {
	_, err := h.pedir(ctx, "PUT", "/ISAPI/System/reboot", "", nil)
	return err
}
