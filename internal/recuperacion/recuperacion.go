package recuperacion

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/almacen"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/descubrir"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/dvr"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/salud"
)

const intervaloMinimo = 10 * time.Minute

type intento struct {
	desde  time.Time
	activo bool
}

type Recuperador struct {
	a        *almacen.Almacen
	estado   func() salud.Estado
	buscar   func(context.Context, descubrir.Opciones) ([]descubrir.Hallazgo, error)
	nuevoDVR func(config.DVR) (dvr.Cliente, error)
	avisar   func(string)
	ahora    func() time.Time
	intentos map[string]intento
}

func Nuevo(a *almacen.Almacen, estado func() salud.Estado, avisar func(string)) *Recuperador {
	return &Recuperador{a: a, estado: estado, buscar: descubrir.Buscar, nuevoDVR: dvr.Nuevo, avisar: avisar, ahora: time.Now, intentos: make(map[string]intento)}
}

func (r *Recuperador) Ejecutar(ctx context.Context) {
	reloj := time.NewTicker(time.Second)
	defer reloj.Stop()
	var grupo sync.WaitGroup
	defer grupo.Wait()
	terminados := make(chan string)
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-terminados:
			i := r.intentos[id]
			i.activo = false
			r.intentos[id] = i
		case <-reloj.C:
			r.revisar(func(s config.Sucursal, red netip.Prefix) {
				grupo.Go(func() {
					limite, fin := context.WithTimeout(ctx, 2*time.Minute)
					defer fin()
					r.recuperar(limite, s, red)
					select {
					case terminados <- s.ID:
					case <-ctx.Done():
					}
				})
			})
		}
	}
}

func (r *Recuperador) revisar(lanzar func(config.Sucursal, netip.Prefix)) {
	cfg, version := r.a.Actual()
	estado := r.estado()
	if estado.ConfigVersion != version {
		return
	}
	ahora := r.ahora()
	for id, i := range r.intentos {
		if !i.activo && ahora.Sub(i.desde) >= intervaloMinimo {
			delete(r.intentos, id)
		}
	}
	caidas := make(map[string]time.Time)
	for _, s := range estado.Sucursales {
		if !s.EnLinea {
			caidas[s.ID] = s.Desde
		}
	}
	umbral := time.Duration(cfg.Salud.RecuperarTrasMinutos) * time.Minute
	for _, s := range cfg.Sucursales {
		desde, caida := caidas[s.ID]
		if !caida || ahora.Sub(desde) < umbral || s.Huella == nil || (s.Huella.MAC == "" && s.Huella.Serie == "") {
			continue
		}
		if _, existe := r.intentos[s.ID]; existe {
			continue
		}
		ip, err := netip.ParseAddr(s.DVR.Host)
		if err != nil || !ip.Is4() || !ip.IsGlobalUnicast() {
			continue
		}
		r.intentos[s.ID] = intento{desde: ahora, activo: true}
		lanzar(s, netip.PrefixFrom(ip, 24).Masked())
	}
}

func puertosCompatibles(d config.DVR, h descubrir.Hallazgo) bool {
	if h.Marca != "" && h.Marca != "onvif" && h.Marca != d.Marca {
		return false
	}
	if d.Puerto != 0 && slices.Contains(h.Puertos, d.Puerto) {
		return true
	}
	switch d.Marca {
	case "hikvision":
		return slices.Contains(h.Puertos, 8000) || slices.Contains(h.Puertos, 80) || slices.Contains(h.Puertos, 554)
	case "dahua":
		return slices.Contains(h.Puertos, 37777) || slices.Contains(h.Puertos, 80) || slices.Contains(h.Puertos, 554)
	case "xmeye":
		return slices.Contains(h.Puertos, 34567) || h.ONVIF
	case "uniview", "onvif":
		return h.ONVIF || slices.Contains(h.Puertos, 80) || slices.Contains(h.Puertos, 8899)
	default:
		return false
	}
}

func mismaHuella(h config.Huella, i dvr.Identidad) bool {
	if h.Serie != "" && strings.EqualFold(strings.TrimSpace(h.Serie), strings.TrimSpace(i.Serie)) && strings.TrimSpace(i.Serie) != "" {
		return true
	}
	original, e1 := net.ParseMAC(h.MAC)
	actual, e2 := net.ParseMAC(i.MAC)
	return e1 == nil && e2 == nil && original.String() == actual.String()
}

func (r *Recuperador) recuperar(ctx context.Context, s config.Sucursal, red netip.Prefix) {
	hallazgos, err := r.buscar(ctx, descubrir.Opciones{Redes: []netip.Prefix{red}, ONVIF: false})
	if err != nil {
		return
	}
	for _, h := range hallazgos {
		if ctx.Err() != nil {
			return
		}
		ip, err := netip.ParseAddr(h.IP)
		if err != nil || !red.Contains(ip) || h.IP == s.DVR.Host || !puertosCompatibles(s.DVR, h) {
			continue
		}
		destino := s.DVR
		destino.Host = h.IP
		cliente, err := r.nuevoDVR(destino)
		if err != nil {
			continue
		}
		identidad, err := cliente.Identificar(ctx)
		if err != nil || ctx.Err() != nil || !mismaHuella(*s.Huella, identidad) {
			continue
		}
		err = r.a.EditarSucursales(func(lista []config.Sucursal) ([]config.Sucursal, error) {
			i := almacen.Indice(lista, s.ID)
			if i < 0 || lista[i].DVR != s.DVR || lista[i].Huella == nil || *lista[i].Huella != *s.Huella {
				return nil, almacen.ErrConflicto
			}
			lista[i].DVR.Host = h.IP
			lista[i].Huella.Actualizada = r.ahora().UTC()
			return lista, nil
		})
		if err == nil {
			r.avisar(fmt.Sprintf("📍 %s: el DVR cambió de %s a %s", s.Nombre, s.DVR.Host, h.IP))
		}
		return
	}
}
