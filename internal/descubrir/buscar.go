package descubrir

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var tailscale = netip.MustParsePrefix("100.64.0.0/10")

func redesLocales() []netip.Prefix {
	interfaces, _ := net.Interfaces()
	redes := map[netip.Prefix]bool{}
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		direcciones, _ := i.Addrs()
		for _, d := range direcciones {
			p, err := netip.ParsePrefix(d.String())
			if err == nil && p.Addr().Is4() && p.Addr().IsPrivate() && !tailscale.Contains(p.Addr()) {
				redes[netip.PrefixFrom(p.Addr(), 24).Masked()] = true
			}
		}
	}
	lista := make([]netip.Prefix, 0, len(redes))
	for p := range redes {
		lista = append(lista, p)
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Addr().Less(lista[j].Addr()) })
	return lista
}

func buscar(ctx context.Context, op Opciones) ([]Hallazgo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	redes := op.Redes
	if len(redes) == 0 {
		redes = RedesLocales()
	}
	rangos := rangosIPv4(redes)
	total := 0
	for _, r := range rangos {
		total += int(r.fin - r.inicio)
	}
	puertos := append([]int(nil), PuertosDVR...)
	sort.Ints(puertos)
	descubrimientos := make(chan []Hallazgo, 1)
	if op.ONVIF {
		go func() { descubrimientos <- descubrirONVIF(ctx) }()
	} else {
		descubrimientos <- nil
	}
	trabajos := make(chan netip.Addr)
	resultados := make(chan Hallazgo)
	var grupo sync.WaitGroup
	for range min(128, total) {
		grupo.Go(func() {
			for ip := range trabajos {
				h := sondear(ctx, ip.String(), puertos)
				resultados <- h
			}
		})
	}
	go func() {
		defer close(trabajos)
		for _, r := range rangos {
			for n := r.inicio; n < r.fin; n++ {
				ip := netip.AddrFrom4([4]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
				select {
				case <-ctx.Done():
					return
				case trabajos <- ip:
				}
			}
		}
	}()
	go func() { grupo.Wait(); close(resultados) }()
	hallazgos := map[string]Hallazgo{}
	hechos := 0
	if op.Progreso != nil {
		op.Progreso(0, total)
	}
	for h := range resultados {
		if len(h.Puertos) > 0 {
			hallazgos[h.IP] = h
		}
		hechos++
		if op.Progreso != nil {
			op.Progreso(hechos, total)
		}
	}
	for _, o := range <-descubrimientos {
		ip := netip.MustParseAddr(o.IP)
		permitido := false
		for _, red := range redes {
			permitido = permitido || red.Contains(ip)
		}
		if !permitido {
			continue
		}
		h := hallazgos[o.IP]
		h.IP, h.ONVIF = o.IP, true
		if h.Marca == "" || h.Marca == "onvif" && o.Marca != "onvif" {
			h.Marca, h.Pista = o.Marca, o.Pista
		}
		if o.Modelo != "" {
			h.Modelo = o.Modelo
		}
		h.Puertos = append(h.Puertos, o.Puertos...)
		sort.Ints(h.Puertos)
		hallazgos[h.IP] = h
	}
	arp := tablaARP(ctx)
	lista := make([]Hallazgo, 0, len(hallazgos))
	for ip, h := range hallazgos {
		h.MAC = arp[ip]
		h.Puertos = slices.Compact(h.Puertos)
		lista = append(lista, h)
	}
	sort.Slice(lista, func(i, j int) bool { return netip.MustParseAddr(lista[i].IP).Less(netip.MustParseAddr(lista[j].IP)) })
	return lista, ctx.Err()
}

// fin es exclusivo. Se usa uint64 para representar el final del espacio IPv4.
type rangoIPv4 struct{ inicio, fin uint64 }

func rangosIPv4(redes []netip.Prefix) []rangoIPv4 {
	rangos := make([]rangoIPv4, 0, len(redes))
	for _, p := range redes {
		if !p.IsValid() || !p.Addr().Is4() {
			continue
		}
		ip := p.Masked().Addr().As4()
		inicio := uint64(binary.BigEndian.Uint32(ip[:]))
		fin := inicio + 1<<(32-p.Bits())
		if p.Bits() < 31 {
			inicio++
			fin--
		}
		rangos = append(rangos, rangoIPv4{inicio, fin})
	}
	sort.Slice(rangos, func(i, j int) bool { return rangos[i].inicio < rangos[j].inicio })
	unidos := rangos[:0]
	for _, r := range rangos {
		i := len(unidos) - 1
		if i >= 0 && r.inicio <= unidos[i].fin {
			unidos[i].fin = max(unidos[i].fin, r.fin)
		} else {
			unidos = append(unidos, r)
		}
	}
	return unidos
}

var httpHuella = &http.Client{
	Timeout:       600 * time.Millisecond,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Transport:     &http.Transport{DisableKeepAlives: true},
}

func sondear(ctx context.Context, ip string, puertos []int) Hallazgo {
	h := Hallazgo{IP: ip}
	dial := net.Dialer{Timeout: 600 * time.Millisecond}
	for _, p := range puertos {
		if ctx.Err() != nil {
			break
		}
		direccion := net.JoinHostPort(ip, strconv.Itoa(p))
		c, err := dial.DialContext(ctx, "tcp", direccion)
		if err != nil {
			continue
		}
		c.Close()
		h.Puertos = append(h.Puertos, p)
		marca, pista := marcaPuerto(p)
		if h.Marca == "" && marca != "" {
			h.Marca, h.Pista = marca, pista
		}
	}
	for _, p := range h.Puertos {
		m, _ := marcaPuerto(p)
		if p == 554 || m != "" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, "GET", "http://"+net.JoinHostPort(ip, strconv.Itoa(p))+"/", nil)
		if err != nil {
			continue
		}
		resp, err := httpHuella.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if marca := marcaTexto(resp.Header.Get("Server") + " " + strings.Join(resp.Header.Values("WWW-Authenticate"), " ")); marca != "" {
			h.Marca, h.Pista = marca, "cabecera HTTP de "+marca
			break
		}
	}
	if h.Pista == "" && len(h.Puertos) > 0 {
		h.Pista = "puertos de DVR abiertos"
	}
	return h
}

func marcaPuerto(p int) (string, string) {
	switch p {
	case 8000:
		return "hikvision", "puerto 8000 (Hikvision)"
	case 37777:
		return "dahua", "puerto 37777 (Dahua)"
	case 34567:
		return "xmeye", "puerto 34567 (XMEye)"
	}
	return "", ""
}

func marcaTexto(s string) string {
	s = strings.ToLower(s)
	for _, m := range []struct {
		marca string
		alias []string
	}{
		{"hikvision", []string{"hikvision", "hik-connect", "webs", "dnvrs", "epcom", "ezviz"}},
		{"dahua", []string{"dahua", "amcrest", "lorex", "imou", "login to"}},
		{"uniview", []string{"uniview", "unv"}},
		{"xmeye", []string{"xmeye", "xiongmai"}},
		{"onvif", []string{"onvif"}},
	} {
		for _, a := range m.alias {
			if strings.Contains(s, a) {
				return m.marca
			}
		}
	}
	return ""
}
