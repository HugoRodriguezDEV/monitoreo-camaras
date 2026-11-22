package descubrir

import (
	"context"
	"crypto/rand"
	"encoding/xml"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func descubrirONVIF(ctx context.Context) []Hallazgo {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	destino := &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 3702}
	id := make([]byte, 16)
	rand.Read(id)
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	mensaje := fmt.Sprintf(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:n="http://www.onvif.org/ver10/network/wsdl"><s:Header><a:MessageID>urn:uuid:%x-%x-%x-%x-%x</a:MessageID><a:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To><a:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action></s:Header><s:Body><d:Probe><d:Types>n:NetworkVideoTransmitter</d:Types></d:Probe></s:Body></s:Envelope>`, id[:4], id[4:6], id[6:8], id[8:10], id[10:])
	resultados := make(chan []Hallazgo)
	var locales []net.IP
	interfaces, _ := net.Interfaces()
	for _, i := range interfaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagMulticast == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		direcciones, _ := i.Addrs()
		for _, d := range direcciones {
			p, err := netip.ParsePrefix(d.String())
			if err == nil && p.Addr().Is4() && !tailscale.Contains(p.Addr()) {
				locales = append(locales, net.IP(p.Addr().AsSlice()))
			}
		}
	}
	if len(locales) == 0 {
		locales = append(locales, nil)
	}
	for _, ip := range locales {
		go func() { resultados <- probe(ctx, ip, destino, []byte(mensaje)) }()
	}
	var lista []Hallazgo
	for range locales {
		lista = append(lista, (<-resultados)...)
	}
	return lista
}

func probe(ctx context.Context, ip net.IP, destino *net.UDPAddr, mensaje []byte) []Hallazgo {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip})
	if err != nil {
		return nil
	}
	defer c.Close()
	fin, _ := ctx.Deadline()
	c.SetDeadline(fin)
	parar := context.AfterFunc(ctx, func() { c.Close() })
	defer parar()
	if _, err = c.WriteToUDP(mensaje, destino); err != nil {
		return nil
	}
	var lista []Hallazgo
	b := make([]byte, 65535)
	for {
		n, _, err := c.ReadFromUDP(b)
		if err != nil {
			return lista
		}
		lista = append(lista, parsearProbe(b[:n])...)
	}
}

func parsearProbe(b []byte) []Hallazgo {
	var sobre struct {
		Coincidencias []struct{ XAddrs, Scopes string } `xml:"Body>ProbeMatches>ProbeMatch"`
	}
	if xml.Unmarshal(b, &sobre) != nil {
		return nil
	}
	var lista []Hallazgo
	for _, c := range sobre.Coincidencias {
		marca, modelo := marcaTexto(c.Scopes), ""
		if marca == "" {
			marca = "onvif"
		}
		for _, s := range strings.Fields(c.Scopes) {
			u, err := url.Parse(s)
			if err != nil || u.Scheme != "onvif" {
				continue
			}
			tipo, valor, _ := strings.Cut(strings.TrimPrefix(u.Path, "/"), "/")
			if tipo == "hardware" || (tipo == "name" && modelo == "") {
				modelo = valor
			}
		}
		for _, s := range strings.Fields(c.XAddrs) {
			u, err := url.Parse(s)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
				continue
			}
			ip, err := netip.ParseAddr(u.Hostname())
			if err != nil || !ip.Is4() {
				continue
			}
			p := 80
			if u.Scheme == "https" {
				p = 443
			}
			if u.Port() != "" {
				p, err = strconv.Atoi(u.Port())
				if err != nil || p < 1 || p > 65535 {
					continue
				}
			}
			lista = append(lista, Hallazgo{IP: ip.String(), Marca: marca, Modelo: modelo, Puertos: []int{p}, ONVIF: true, Pista: "ONVIF"})
		}
	}
	return lista
}
