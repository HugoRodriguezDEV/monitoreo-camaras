package dvr

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/dvr/digest"
)

type equipo struct {
	config.DVR
	base string
}

func nuevo(d config.DVR) (Cliente, error) {
	puerto := d.Puerto
	if puerto == 0 {
		puerto = 80
	}
	e := equipo{d, "http://" + net.JoinHostPort(strings.Trim(d.Host, "[]"), strconv.Itoa(puerto))}
	switch d.Marca {
	case "hikvision":
		return hikvision{e}, nil
	case "dahua":
		return dahua{e}, nil
	case "onvif", "xmeye", "uniview":
		return onvif{e}, nil
	case "rtsp":
		return rtsp{}, nil
	default:
		return nil, fmt.Errorf("marca %q: %w", d.Marca, ErrNoSoportado)
	}
}

type rtsp struct{}

func (rtsp) Identificar(context.Context) (Identidad, error) {
	return Identidad{Marca: "rtsp", Canales: []Canal{}}, nil
}
func (rtsp) Foto(context.Context, int) ([]byte, error) { return nil, ErrNoSoportado }
func (rtsp) Reiniciar(context.Context) error           { return ErrNoSoportado }

var clienteHTTP = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type estadoHTTP int

func (e estadoHTTP) Error() string { return fmt.Sprintf("el DVR respondió HTTP %d", e) }

func errorRed(err error) error {
	var ne net.Error
	var op *net.OpError
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, context.DeadlineExceeded) ||
		(errors.As(err, &ne) && ne.Timeout()) ||
		(!errors.Is(err, context.Canceled) && errors.As(err, &op) && op.Op == "dial") {
		return fmt.Errorf("%w: %w", ErrInalcanzable, err)
	}
	return fmt.Errorf("error de comunicación con el DVR: %w", err)
}

func (e equipo) pedir(ctx context.Context, metodo, ruta, tipo string, cuerpo []byte) ([]byte, error) {
	ctx, cancelar := context.WithTimeout(ctx, 5*time.Second)
	defer cancelar()
	base, err := url.Parse(e.base)
	if err != nil {
		return nil, fmt.Errorf("dirección del DVR inválida: %w", err)
	}
	destino, err := base.Parse(ruta)
	if err != nil {
		return nil, fmt.Errorf("dirección del DVR inválida: %w", err)
	}
	destino.User = nil
	req, err := http.NewRequestWithContext(ctx, metodo, destino.String(), bytes.NewReader(cuerpo))
	if err != nil {
		return nil, fmt.Errorf("solicitud al DVR inválida: %w", err)
	}
	if tipo != "" {
		req.Header.Set("Content-Type", tipo)
	}
	resp, err := clienteHTTP.Do(req)
	if err != nil {
		return nil, errorRed(err)
	}
	if resp.StatusCode == 401 {
		var auth string
		for _, reto := range resp.Header.Values("WWW-Authenticate") {
			auth = digest.Autorizacion(reto, metodo, req.URL.RequestURI(), e.Usuario, e.Password, rand.Text())
			if auth != "" {
				break
			}
		}
		if auth != "" {
			resp.Body.Close()
			req = req.Clone(ctx)
			req.Body, _ = req.GetBody()
			req.Header.Set("Authorization", auth)
			resp, err = clienteHTTP.Do(req)
			if err != nil {
				return nil, errorRed(err)
			}
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 {
		return nil, ErrCredenciales
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if strings.HasPrefix(tipo, "application/soap+xml") {
			b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if err != nil {
				return nil, errorRed(err)
			}
			return b, estadoHTTP(resp.StatusCode)
		}
		return nil, estadoHTTP(resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, errorRed(err)
	}
	return b, nil
}

func leerXML(b []byte, v any) error {
	if err := xml.Unmarshal(b, v); err != nil {
		return fmt.Errorf("respuesta XML del DVR inválida: %w", err)
	}
	return nil
}

func mac(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 12 {
		s = strings.Join([]string{s[:2], s[2:4], s[4:6], s[6:8], s[8:10], s[10:]}, ":")
	}
	if m, err := net.ParseMAC(s); err == nil && len(m) == 6 {
		return m.String()
	}
	return ""
}
func codec(s string) string {
	switch strings.ToUpper(strings.NewReplacer(".", "", "-", "", " ", "").Replace(s)) {
	case "H264", "AVC":
		return "H.264"
	case "H265", "HEVC":
		return "H.265"
	case "MJPEG", "JPEG":
		return "MJPEG"
	}
	return ""
}
func resolucion(w, h int) string {
	if w > 0 && h > 0 {
		return fmt.Sprintf("%dx%d", w, h)
	}
	return ""
}
func nombreCanal(n int, nombre string) string {
	if nombre != "" {
		return nombre
	}
	return fmt.Sprintf("Canal %d", n)
}
