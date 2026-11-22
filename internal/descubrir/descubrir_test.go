package descubrir

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestBuscar(t *testing.T) {
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	go func() {
		for {
			c, err := tcp.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	for _, cabecera := range []string{"Server", "WWW-Authenticate"} {
		t.Run(cabecera, func(t *testing.T) {
			httpDVR := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" {
					t.Error("el barrido envió credenciales")
				}
				if cabecera == "Server" {
					w.Header().Set("Server", "Hikvision-Webs")
				} else {
					w.Header().Set("WWW-Authenticate", `Digest realm="Login to XVR", nonce="abc"`)
				}
				w.WriteHeader(401)
			}))
			defer httpDVR.Close()
			anterior := PuertosDVR
			PuertosDVR = []int{tcp.Addr().(*net.TCPAddr).Port, httpDVR.Listener.Addr().(*net.TCPAddr).Port}
			t.Cleanup(func() { PuertosDVR = anterior })
			var progreso []int
			hallazgos, err := Buscar(context.Background(), Opciones{
				Redes: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
				Progreso: func(hechos, total int) {
					if total != 1 {
						t.Errorf("total: %d", total)
					}
					progreso = append(progreso, hechos)
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			marca := "hikvision"
			if cabecera == "WWW-Authenticate" {
				marca = "dahua"
			}
			if len(hallazgos) != 1 || hallazgos[0].IP != "127.0.0.1" || len(hallazgos[0].Puertos) != 2 ||
				hallazgos[0].Marca != marca || hallazgos[0].Pista != "cabecera HTTP de "+marca || !reflect.DeepEqual(progreso, []int{0, 1}) {
				t.Fatalf("hallazgos: %+v, progreso: %v", hallazgos, progreso)
			}
		})
	}
	for puerto, marca := range map[int]string{8000: "hikvision", 37777: "dahua", 34567: "xmeye"} {
		if m, _ := marcaPuerto(puerto); m != marca {
			t.Errorf("puerto %d: %s", puerto, m)
		}
	}
}

func TestBuscarCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Buscar(ctx, Opciones{ONVIF: true, Redes: []netip.Prefix{netip.MustParsePrefix("192.168.1.0/24")}}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func fixture(t *testing.T, nombre string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + nombre)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestARP(t *testing.T) {
	for _, sistema := range []string{"windows", "linux", "darwin"} {
		t.Run(sistema, func(t *testing.T) {
			tabla := parsearARP(string(fixture(t, "arp-"+sistema+".txt")), sistema)
			if !reflect.DeepEqual(tabla, map[string]string{"192.168.1.108": "02:0a:8c:12:34:56"}) {
				t.Fatalf("tabla: %v", tabla)
			}
		})
	}
}

func TestProbeMatch(t *testing.T) {
	lista := parsearProbe(fixture(t, "probe-match.xml"))
	esperado := Hallazgo{IP: "192.168.1.108", Marca: "uniview", Modelo: "NVR 301", Puertos: []int{8899}, ONVIF: true, Pista: "ONVIF"}
	if !reflect.DeepEqual(lista, []Hallazgo{esperado}) {
		t.Fatalf("hallazgos: %+v", lista)
	}
}

func TestRedesLocales(t *testing.T) {
	for _, p := range RedesLocales() {
		if p.Bits() != 24 || !p.Addr().Is4() || !p.Addr().IsPrivate() || tailscale.Contains(p.Addr()) || p != p.Masked() {
			t.Errorf("red local inválida: %s", p)
		}
	}
}

func TestProbeUDP(t *testing.T) {
	s, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	respuesta := fixture(t, "probe-match.xml")
	go func() {
		b := make([]byte, 256)
		_, remoto, err := s.ReadFromUDP(b)
		if err == nil {
			s.WriteToUDP(respuesta, remoto)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	lista := probe(ctx, net.IPv4(127, 0, 0, 1), s.LocalAddr().(*net.UDPAddr), []byte("<Probe/>"))
	if len(lista) != 1 || lista[0].IP != "192.168.1.108" {
		t.Fatalf("hallazgos UDP: %+v", lista)
	}
}
