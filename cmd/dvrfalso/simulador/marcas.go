package simulador

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
)

func escapar(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

var fotoHik = regexp.MustCompile(`^/ISAPI/Streaming/channels/(\d+)01/picture$`)
var flujoHik = regexp.MustCompile(`^/ISAPI/Streaming/channels/(\d+)02$`)

func (s servidor) hikvision(w http.ResponseWriter, r *http.Request) {
	ruta := r.URL.Path
	if r.Method == "GET" {
		if m := fotoHik.FindStringSubmatch(ruta); m != nil {
			s.foto(w, m[1])
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		switch ruta {
		case "/ISAPI/System/deviceInfo":
			fmt.Fprintf(w, "<DeviceInfo><model>DS-SIMULADO</model><serialNumber>%s</serialNumber><macAddress>%s</macAddress><firmwareVersion>V4.0.0</firmwareVersion></DeviceInfo>", escapar(s.Serie), escapar(s.MAC))
			return
		case "/ISAPI/System/Network/interfaces":
			tipo := "static"
			if s.DHCP {
				tipo = "dhcp"
			}
			fmt.Fprintf(w, "<NetworkInterfaceList><NetworkInterface><IPAddress><addressingType>%s</addressingType></IPAddress><Link><MACAddress>%s</MACAddress></Link></NetworkInterface></NetworkInterfaceList>", tipo, escapar(s.MAC))
			return
		case "/ISAPI/ContentMgmt/InputProxy/channels", "/ISAPI/System/Video/inputs/channels":
			etiqueta := "InputProxyChannel"
			if ruta == "/ISAPI/System/Video/inputs/channels" {
				etiqueta = "VideoInputChannel"
			}
			fmt.Fprintf(w, "<%sList>", etiqueta)
			for n := 1; n <= s.Canales; n++ {
				fmt.Fprintf(w, "<%s><id>%d</id><name>Canal %d</name><online>true</online></%s>", etiqueta, n, n, etiqueta)
			}
			fmt.Fprintf(w, "</%sList>", etiqueta)
			return
		}
		if m := flujoHik.FindStringSubmatch(ruta); m != nil {
			n, _ := strconv.Atoi(m[1])
			if n >= 1 && n <= s.Canales {
				fmt.Fprintf(w, "<StreamingChannel><Video><videoCodecType>%s</videoCodecType><videoResolutionWidth>640</videoResolutionWidth><videoResolutionHeight>360</videoResolutionHeight><maxFrameRate>1500</maxFrameRate></Video></StreamingChannel>", s.codec(n))
				return
			}
		}
	}
	if r.Method == "PUT" && ruta == "/ISAPI/System/reboot" {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, "<ResponseStatus><statusCode>1</statusCode><statusString>OK</statusString></ResponseStatus>")
		return
	}
	http.NotFound(w, r)
}

func (s servidor) dahua(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if r.Method != "GET" {
		http.NotFound(w, r)
		return
	}
	if r.URL.Path == "/cgi-bin/snapshot.cgi" {
		s.foto(w, q.Get("channel"))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	switch r.URL.Path {
	case "/cgi-bin/magicBox.cgi":
		switch q.Get("action") {
		case "getSystemInfo":
			fmt.Fprintf(w, "deviceType=XVR-SIMULADO\r\nserialNumber=%s\r\nversion=4.0.0\r\n", s.Serie)
		case "getSerialNo":
			fmt.Fprintf(w, "sn=%s\r\n", s.Serie)
		case "getDeviceType":
			fmt.Fprint(w, "type=XVR-SIMULADO\r\n")
		case "getSoftwareVersion":
			fmt.Fprint(w, "version=4.0.0\r\n")
		case "reboot":
			fmt.Fprint(w, "OK\r\n")
		default:
			http.NotFound(w, r)
		}
	case "/cgi-bin/configManager.cgi":
		if q.Get("action") != "getConfig" {
			http.NotFound(w, r)
			return
		}
		switch q.Get("name") {
		case "Network":
			fmt.Fprintf(w, "table.Network.DefaultInterface=eth0\r\ntable.Network.eth0.PhysicalAddress=%s\r\ntable.Network.eth0.DhcpEnable=%t\r\n", s.MAC, s.DHCP)
		case "ChannelTitle":
			for i := 0; i < s.Canales; i++ {
				fmt.Fprintf(w, "table.ChannelTitle[%d].Name=Canal %d\r\n", i, i+1)
			}
		case "Encode":
			for i := 0; i < s.Canales; i++ {
				p := fmt.Sprintf("table.Encode[%d].ExtraFormat[0].Video.", i)
				fmt.Fprintf(w, "%sCompression=%s\r\n%sWidth=640\r\n%sHeight=360\r\n%sFPS=15\r\n", p, s.codec(i+1), p, p, p)
			}
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}
