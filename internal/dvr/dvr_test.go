package dvr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/jpeg"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/cmd/dvrfalso/simulador"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

func clientePrueba(t *testing.T, marca, direccion, clave string) Cliente {
	t.Helper()
	host, p, err := net.SplitHostPort(strings.TrimPrefix(direccion, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	puerto, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Nuevo(config.DVR{Marca: marca, Host: host, PuertoWeb: puerto, Usuario: `admin"`, Password: clave})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClientesSimulados(t *testing.T) {
	for _, marca := range []string{"hikvision", "dahua", "onvif", "uniview", "xmeye"} {
		t.Run(marca, func(t *testing.T) {
			protocolo := marca
			if marca == "uniview" || marca == "xmeye" {
				protocolo = "onvif"
			}
			op := simulador.Opciones{Marca: protocolo, Usuario: `admin"`, Password: "clave<&", MAC: "AA-BB-CC-DD-EE-FF", Serie: "SIM-123", Canales: 4, H265: map[int]bool{3: true}, DHCP: true}
			var reinicios atomic.Int32
			handler := simulador.Nuevo(op)
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				grabador := httptest.NewRecorder()
				handler.ServeHTTP(grabador, r)
				if grabador.Code == 200 && (strings.Contains(r.URL.Path, "reboot") || r.URL.Query().Get("action") == "reboot" ||
					strings.Contains(r.Header.Get("Content-Type"), "/SystemReboot")) {
					reinicios.Add(1)
				}
				for k, v := range grabador.Header() {
					w.Header()[k] = v
				}
				w.WriteHeader(grabador.Code)
				w.Write(grabador.Body.Bytes())
			}))
			defer s.Close()
			ctx := context.Background()
			malo := clientePrueba(t, marca, s.URL, "incorrecta")
			if _, err := malo.Identificar(ctx); !errors.Is(err, ErrCredenciales) {
				t.Fatalf("credenciales: %v", err)
			}
			c := clientePrueba(t, marca, s.URL, op.Password)
			id, err := c.Identificar(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if id.Marca != marca || id.Modelo == "" || id.Serie != op.Serie || id.MAC != "aa:bb:cc:dd:ee:ff" || id.Firmware == "" || id.DHCP == nil || !*id.DHCP || len(id.Canales) != 4 {
				t.Fatalf("identidad: %+v", id)
			}
			for i, canal := range id.Canales {
				codec := "H.264"
				if i == 2 {
					codec = "H.265"
				}
				if canal.Canal != i+1 || canal.CodecSub != codec || canal.ResolucionSub != "640x360" || canal.FpsSub != 15 || !canal.Conectado {
					t.Fatalf("canal: %+v", canal)
				}
			}
			b, err := c.Foto(ctx, 3)
			if err != nil {
				t.Fatal(err)
			}
			foto, err := jpeg.Decode(bytes.NewReader(b))
			if err != nil || foto.Bounds().Dx() != 320 {
				t.Fatalf("foto inválida: %v", err)
			}
			if err := c.Reiniciar(ctx); err != nil {
				t.Fatal(err)
			}
			if reinicios.Load() != 1 {
				t.Fatalf("reinicios: %d", reinicios.Load())
			}
		})
	}
}

func TestSinONVIF(t *testing.T) {
	s := httptest.NewServer(http.NotFoundHandler())
	defer s.Close()
	for _, marca := range []string{"uniview", "xmeye"} {
		_, err := clientePrueba(t, marca, s.URL, "").Identificar(context.Background())
		if !errors.Is(err, ErrNoSoportado) || !strings.Contains(err.Error(), "a mano") {
			t.Fatalf("%s: %v", marca, err)
		}
	}
}

func TestFalloSOAPCredenciales(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><s:Fault><s:Code><s:Value>s:Sender</s:Value><s:Subcode><s:Value>ter:NotAuthorized</s:Value></s:Subcode></s:Code></s:Fault></s:Body></s:Envelope>`))
	}))
	defer s.Close()
	if _, err := clientePrueba(t, "onvif", s.URL, "").Identificar(context.Background()); !errors.Is(err, ErrCredenciales) {
		t.Fatalf("fallo SOAP: %v", err)
	}
}

func TestInalcanzable(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	direccion := l.Addr().String()
	l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := clientePrueba(t, "hikvision", direccion, "").Identificar(ctx); !errors.Is(err, ErrInalcanzable) {
		t.Fatalf("conexión rechazada: %v", err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
		}
	}))
	defer s.Close()
	limite, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := clientePrueba(t, "hikvision", s.URL, "").Identificar(limite); !errors.Is(err, ErrInalcanzable) {
		t.Fatalf("tiempo agotado: %v", err)
	}
}

func TestRTSP(t *testing.T) {
	c, err := Nuevo(config.DVR{Marca: "rtsp"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, err := c.Identificar(ctx)
	if err != nil || id.Marca != "rtsp" || id.Canales == nil || len(id.Canales) != 0 {
		t.Fatalf("identidad: %+v, %v", id, err)
	}
	if _, err := c.Foto(ctx, 1); !errors.Is(err, ErrNoSoportado) {
		t.Fatal(err)
	}
	if err := c.Reiniciar(ctx); !errors.Is(err, ErrNoSoportado) {
		t.Fatal(err)
	}
}

// Conecta al ejecutable lanzado por separado para verificar también sus flags.
func TestDVRFalsoEjecutable(t *testing.T) {
	direccion, marca := os.Getenv("DVRFALSO_DIRECCION"), os.Getenv("DVRFALSO_MARCA")
	if direccion == "" || marca == "" {
		t.Skip("sin ejecutable de prueba externo")
	}
	host, p, err := net.SplitHostPort(direccion)
	if err != nil {
		t.Fatal(err)
	}
	puerto, err := strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Nuevo(config.DVR{Marca: marca, Host: host, PuertoWeb: puerto, Usuario: "admin", Password: "admin123"})
	if err != nil {
		t.Fatal(err)
	}
	id, err := c.Identificar(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id.Marca != marca || id.DHCP == nil || !*id.DHCP || len(id.Canales) != 4 || id.Canales[2].CodecSub != "H.265" {
		t.Fatalf("identidad inesperada: %+v", id)
	}
	b, err := json.Marshal(id)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(string(b))
}
