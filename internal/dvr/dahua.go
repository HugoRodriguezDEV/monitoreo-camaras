package dvr

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type dahua struct{ equipo }

func camposCGI(b []byte) map[string]string {
	campos := map[string]string{}
	for _, linea := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(linea, "=")
		if ok {
			campos[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return campos
}

func (d dahua) leer(ctx context.Context, ruta string) (map[string]string, error) {
	b, err := d.pedir(ctx, "GET", "/cgi-bin/"+ruta, "", nil)
	if err != nil {
		return nil, err
	}
	return camposCGI(b), nil
}

var campoCanalCGI = regexp.MustCompile(`^table\.(?:Encode|ChannelTitle)\[(\d+)\]\.`)

func (d dahua) Identificar(ctx context.Context) (Identidad, error) {
	info, err := d.leer(ctx, "magicBox.cgi?action=getSystemInfo")
	if err != nil && !opcional(err) {
		return Identidad{}, err
	}
	id := Identidad{Marca: d.Marca, Modelo: info["deviceType"], Serie: info["serialNumber"], Firmware: info["version"], Canales: []Canal{}}
	for _, c := range []struct {
		accion, clave string
		valor         *string
	}{
		{"getDeviceType", "type", &id.Modelo}, {"getSerialNo", "sn", &id.Serie}, {"getSoftwareVersion", "version", &id.Firmware},
	} {
		if *c.valor != "" {
			continue
		}
		m, err := d.leer(ctx, "magicBox.cgi?action="+c.accion)
		if err != nil && !opcional(err) {
			return Identidad{}, err
		}
		*c.valor = m[c.clave]
	}
	red, err := d.leer(ctx, "configManager.cgi?action=getConfig&name=Network")
	if err != nil && !opcional(err) {
		return Identidad{}, err
	}
	interfaz := red["table.Network.DefaultInterface"]
	if interfaz == "" {
		host := net.ParseIP(strings.Trim(d.Host, "[]"))
		for _, k := range clavesCGI(red) {
			if strings.HasSuffix(k, ".IPAddress") && host != nil && host.Equal(net.ParseIP(red[k])) {
				interfaz = strings.TrimSuffix(strings.TrimPrefix(k, "table.Network."), ".IPAddress")
				break
			}
		}
	}
	prefijo := "table.Network." + interfaz + "."
	if interfaz == "" {
		for _, k := range clavesCGI(red) {
			if strings.HasSuffix(k, ".PhysicalAddress") || strings.HasSuffix(k, ".MACAddress") {
				prefijo = k[:strings.LastIndex(k, ".")+1]
				break
			}
		}
	}
	id.MAC = mac(red[prefijo+"PhysicalAddress"])
	if id.MAC == "" {
		id.MAC = mac(red[prefijo+"MACAddress"])
	}
	if dhcp, err := strconv.ParseBool(red[prefijo+"DhcpEnable"]); err == nil {
		id.DHCP = &dhcp
	}
	titulos, err := d.leer(ctx, "configManager.cgi?action=getConfig&name=ChannelTitle")
	if err != nil && !opcional(err) {
		return Identidad{}, err
	}
	enc, err := d.leer(ctx, "configManager.cgi?action=getConfig&name=Encode")
	if err != nil && !opcional(err) {
		return Identidad{}, err
	}
	indices := map[int]bool{}
	for _, m := range []map[string]string{titulos, enc} {
		for k := range m {
			if p := campoCanalCGI.FindStringSubmatch(k); p != nil {
				if n, err := strconv.Atoi(p[1]); err == nil {
					indices[n] = true
				}
			}
		}
	}
	orden := make([]int, 0, len(indices))
	for n := range indices {
		orden = append(orden, n)
	}
	sort.Ints(orden)
	for _, n := range orden {
		p := fmt.Sprintf("table.Encode[%d].ExtraFormat[0].Video.", n)
		ancho, _ := strconv.Atoi(enc[p+"Width"])
		alto, _ := strconv.Atoi(enc[p+"Height"])
		fps, _ := strconv.ParseFloat(enc[p+"FPS"], 64)
		nombre := titulos[fmt.Sprintf("table.ChannelTitle[%d].Name", n)]
		id.Canales = append(id.Canales, Canal{n + 1, nombreCanal(n+1, nombre), codec(enc[p+"Compression"]), resolucion(ancho, alto), int(fps), true})
	}
	return id, nil
}

func clavesCGI(m map[string]string) []string {
	claves := make([]string, 0, len(m))
	for k := range m {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	return claves
}

func (d dahua) Foto(ctx context.Context, canal int) ([]byte, error) {
	return d.pedir(ctx, "GET", fmt.Sprintf("/cgi-bin/snapshot.cgi?channel=%d", canal), "", nil)
}
func (d dahua) Reiniciar(ctx context.Context) error {
	_, err := d.pedir(ctx, "GET", "/cgi-bin/magicBox.cgi?action=reboot", "", nil)
	return err
}
