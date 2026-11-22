// Package dvr habla con la API de cada marca de DVR: identificarlo (marca, modelo,
// serie, MAC, canales), sacar una foto de un canal y reiniciarlo.
//
// Contrato compartido (docs/V2.md). Las firmas de este archivo no cambian sin
// actualizar el contrato; la implementación vive en los demás archivos del paquete.
package dvr

import (
	"context"
	"errors"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

// Canal describe un canal del DVR tal como lo reporta su API.
type Canal struct {
	Canal  int    `json:"canal"`
	Nombre string `json:"nombre"`
	// Codec del substream: "H.264", "H.265", "MJPEG" o "" si no se pudo leer.
	CodecSub      string `json:"codecSub"`
	ResolucionSub string `json:"resolucionSub"` // "640x360" o ""
	FpsSub        int    `json:"fpsSub"`
	// Conectado: el canal tiene una cámara con señal. Si la API no lo dice, true.
	Conectado bool `json:"conectado"`
}

// Identidad es la "huella" del DVR. MAC y Serie permiten volver a encontrarlo si
// cambia de IP. Cualquier campo puede venir vacío si la marca no lo expone.
type Identidad struct {
	Marca    string  `json:"marca"` // hikvision | dahua | uniview | xmeye | onvif | rtsp
	Modelo   string  `json:"modelo"`
	Serie    string  `json:"serie"`
	MAC      string  `json:"mac"` // minúsculas, separada por ":" (aa:bb:cc:dd:ee:ff)
	Firmware string  `json:"firmware"`
	Canales  []Canal `json:"canales"`
	// DHCP: true si el DVR toma su IP del módem (conviene fijarla). nil = desconocido.
	DHCP *bool `json:"dhcp"`
}

var (
	// ErrCredenciales: el DVR respondió pero rechazó usuario o contraseña.
	ErrCredenciales = errors.New("usuario o contraseña incorrectos")
	// ErrNoSoportado: la marca no expone esa operación por API.
	ErrNoSoportado = errors.New("la marca no permite esta operación por API")
	// ErrInalcanzable: no hubo respuesta del DVR.
	ErrInalcanzable = errors.New("el DVR no responde")
)

// Cliente habla con un DVR concreto usando las credenciales de config.DVR.
type Cliente interface {
	Identificar(ctx context.Context) (Identidad, error)
	// Foto devuelve un JPEG del canal (substream si se puede). Sirve de vista previa
	// en el asistente de instalación, donde no hay go2rtc.
	Foto(ctx context.Context, canal int) ([]byte, error)
	Reiniciar(ctx context.Context) error
}

// Nuevo regresa el cliente de la marca de d.Marca. Para "xmeye" y "uniview" usa
// ONVIF si el DVR lo tiene; para "rtsp" regresa un cliente cuyas operaciones dan
// ErrNoSoportado salvo Identificar, que responde lo mínimo (marca y canales configurados
// no aplican: lista vacía).
var Nuevo func(d config.DVR) (Cliente, error) = nuevoNoImplementado

func nuevoNoImplementado(config.DVR) (Cliente, error) {
	return nil, ErrNoSoportado
}
