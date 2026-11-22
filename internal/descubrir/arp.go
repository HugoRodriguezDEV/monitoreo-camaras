package descubrir

import (
	"context"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func tablaARP(ctx context.Context) map[string]string {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var b []byte
	switch runtime.GOOS {
	case "linux":
		b, _ = os.ReadFile("/proc/net/arp")
	case "windows":
		b, _ = exec.CommandContext(ctx, "arp", "-a").Output()
	case "darwin":
		b, _ = exec.CommandContext(ctx, "arp", "-an").Output()
	}
	return parsearARP(string(b), runtime.GOOS)
}

func parsearARP(s, sistema string) map[string]string {
	tabla := map[string]string{}
	for _, l := range strings.Split(s, "\n") {
		c := strings.Fields(l)
		var ip, mac string
		switch sistema {
		case "linux":
			if len(c) < 4 || c[2] != "0x2" {
				continue
			}
			ip, mac = c[0], c[3]
		case "windows":
			if len(c) < 2 {
				continue
			}
			ip, mac = c[0], c[1]
		case "darwin":
			if len(c) < 4 || c[2] != "at" {
				continue
			}
			ip, mac = strings.Trim(c[1], "()"), c[3]
			partes := strings.Split(mac, ":")
			for i, p := range partes {
				if len(p) == 1 {
					partes[i] = "0" + p
				}
			}
			mac = strings.Join(partes, ":")
		}
		a, err := netip.ParseAddr(ip)
		if err != nil || !a.Is4() || a.IsMulticast() || tailscale.Contains(a) {
			continue
		}
		m, err := net.ParseMAC(mac)
		if err == nil && len(m) == 6 && m[0]&1 == 0 && m.String() != "00:00:00:00:00:00" {
			tabla[a.String()] = m.String()
		}
	}
	return tabla
}
