// Package descubrir busca DVR en una red sin credenciales: barrido de puertos,
// ONVIF WS-Discovery, huella HTTP y tabla ARP para la MAC.
//
// Contrato compartido (docs/V2.md). Las firmas de este archivo no cambian sin
// actualizar el contrato; la implementación vive en los demás archivos del paquete.
package descubrir

import (
	"context"
	"net/netip"
)

// Hallazgo es un equipo que parece DVR o cámara.
type Hallazgo struct {
	IP  string `json:"ip"`
	MAC string `json:"mac"` // solo en la red local (ARP); "" a través de Tailscale
	// Marca probable sin credenciales: hikvision | dahua | uniview | xmeye | onvif | "".
	Marca   string `json:"marca"`
	Modelo  string `json:"modelo"` // si lo anuncia ONVIF o el banner HTTP
	Puertos []int  `json:"puertos"`
	ONVIF   bool   `json:"onvif"`
	// Motivo legible: "ONVIF", "puerto 37777 (Dahua)", "cabecera HTTP de Hikvision"…
	Pista string `json:"pista"`
}

// Opciones del barrido.
type Opciones struct {
	// Redes a barrer. Vacío = las redes IPv4 privadas de las interfaces locales,
	// recortadas a su /24.
	Redes []netip.Prefix
	// ONVIF WS-Discovery por multicast (solo sirve en la red local).
	ONVIF bool
	// Progreso opcional: hechos de total direcciones.
	Progreso func(hechos, total int)
}

// PuertosDVR son los puertos que se prueban en cada dirección.
var PuertosDVR = []int{80, 554, 8000, 8899, 34567, 37777}

// Buscar barre las redes y regresa los hallazgos ordenados por IP. Respeta ctx.
var Buscar func(ctx context.Context, op Opciones) ([]Hallazgo, error) = buscar

// RedesLocales regresa las redes IPv4 privadas de las interfaces de esta máquina (/24).
var RedesLocales func() []netip.Prefix = redesLocales
