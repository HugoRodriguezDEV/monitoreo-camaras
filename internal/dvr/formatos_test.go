package dvr

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Sirve las respuestas de formato real sin usar el generador del simulador.
func TestFormatos(t *testing.T) {
	for _, marca := range []string{"hikvision", "dahua", "onvif"} {
		t.Run(marca, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				switch {
				case r.URL.Path == "/ISAPI/System/deviceInfo":
					http.ServeFile(w, r, "testdata/isapi-equipo.xml")
				case r.URL.Path == "/ISAPI/System/Video/inputs/channels":
					http.ServeFile(w, r, "testdata/isapi-entradas.xml")
				case r.URL.Path == "/ISAPI/Streaming/channels/102":
					http.ServeFile(w, r, "testdata/isapi-stream.xml")
				case q.Get("name") == "Network":
					http.ServeFile(w, r, "testdata/dahua-red.txt")
				case q.Get("name") == "Encode":
					http.ServeFile(w, r, "testdata/dahua-encode.txt")
				case q.Get("name") == "ChannelTitle":
					io.WriteString(w, "table.ChannelTitle[0].Name=Entrada\r\n")
				case q.Get("action") == "getSystemInfo":
					io.WriteString(w, "processor=SSC337\r\n")
				case q.Get("action") == "getDeviceType":
					io.WriteString(w, "type=XVR5104HS-I3\r\n")
				case q.Get("action") == "getSerialNo":
					io.WriteString(w, "sn=7A0271\r\n")
				case q.Get("action") == "getSoftwareVersion":
					io.WriteString(w, "version=4.001.0000000.1\r\n")
				case strings.Contains(r.Header.Get("Content-Type"), "/GetDeviceInformation"):
					io.WriteString(w, `<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetDeviceInformationResponse><Model>NVR-104</Model><SerialNumber>789</SerialNumber><FirmwareVersion>1.2</FirmwareVersion></GetDeviceInformationResponse></s:Body></s:Envelope>`)
				case r.URL.Path == "/onvif/media_service":
					b, err := os.ReadFile("testdata/onvif-perfiles.xml")
					if err != nil {
						http.Error(w, err.Error(), 500)
						return
					}
					w.Write(b)
				default:
					http.NotFound(w, r)
				}
			}))
			defer s.Close()
			id, err := clientePrueba(t, marca, s.URL, "").Identificar(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(id.Canales) == 0 || id.Canales[0].Nombre != "Entrada" || id.Canales[0].CodecSub != "H.265" {
				t.Fatalf("identidad: %+v", id)
			}
			switch marca {
			case "hikvision":
				if id.Modelo != "DS-7204HQHI-K1" || id.Serie != "DS7204HQHI20230101AAWR000001" || id.MAC != "84:9a:40:12:34:56" || id.Canales[0].FpsSub != 25 || id.Canales[0].ResolucionSub != "704x576" {
					t.Fatalf("ISAPI: %+v", id)
				}
			case "dahua":
				if id.Modelo != "XVR5104HS-I3" || id.Serie != "7A0271" || id.DHCP == nil || *id.DHCP || id.MAC != "3c:ef:8c:12:34:56" || len(id.Canales) != 2 || id.Canales[1].Canal != 3 || id.Canales[0].ResolucionSub != "704x480" {
					t.Fatalf("CGI: %+v", id)
				}
			case "onvif":
				if id.Modelo != "NVR-104" || len(id.Canales) != 2 || id.Canales[0].ResolucionSub != "640x360" || id.Canales[1].CodecSub != "MJPEG" {
					t.Fatalf("ONVIF: %+v", id)
				}
			}
		})
	}
}
