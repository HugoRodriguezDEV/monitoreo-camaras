package config

import (
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Datos      string
	Servidor   Servidor
	Go2rtc     Go2rtc
	Interfaz   Interfaz
	Salud      Salud
	Alertas    Alertas
	Sucursales []Sucursal
}
type Servidor struct{ Escuchar string }
type Go2rtc struct {
	Binario string
	API     string
	Config  string
	Externo bool
}
type Interfaz struct {
	RotacionSegundos          int  `yaml:"rotacion_segundos" json:"rotacionSegundos"`
	RotarVistaGeneral         bool `yaml:"rotar_vista_general" json:"rotarVistaGeneral"`
	VolverACuadriculaSegundos int  `yaml:"volver_a_cuadricula_segundos" json:"volverACuadriculaSegundos"`
}
type Salud struct {
	IntervaloSegundos    int `yaml:"intervalo_segundos"`
	CaidoTrasFallos      int `yaml:"caido_tras_fallos"`
	RecuperarTrasMinutos int `yaml:"recuperar_tras_minutos"`
}
type Alertas struct {
	Telegram Telegram `json:"telegram"`
}
type Telegram struct {
	Token  string `json:"token"`
	ChatID string `yaml:"chat_id" json:"chatId"`
}
type Sucursal struct {
	ID      string   `json:"id"`
	Nombre  string   `json:"nombre"`
	DVR     DVR      `json:"dvr"`
	Camaras []Camara `json:"camaras"`
	Huella  *Huella  `json:"huella,omitempty"`
}
type Huella struct {
	MAC         string    `json:"mac"`
	Serie       string    `json:"serie"`
	Modelo      string    `json:"modelo"`
	Marca       string    `json:"marca"`
	Actualizada time.Time `json:"actualizada"`
}

type DVR struct {
	Marca         string `json:"marca"`
	Host          string `json:"host"`
	Puerto        int    `json:"puerto"`
	Usuario       string `json:"usuario"`
	Password      string `json:"password"`
	RutaSub       string `yaml:"ruta_sub" json:"rutaSub"`
	RutaPrincipal string `yaml:"ruta_principal" json:"rutaPrincipal"`
}
type Camara struct {
	Canal  int    `json:"canal"`
	Nombre string `json:"nombre"`
	Clave  bool   `json:"clave"`
}

func Cargar(ruta string) (Config, error) {
	cfg, err := CargarSistema(ruta)
	if err != nil {
		return cfg, err
	}
	return cfg, ValidarSucursales(cfg.Sucursales)
}

// CargarSistema delega la validación de sucursales al almacén durante la migración.
func CargarSistema(ruta string) (Config, error) {
	archivo, err := os.Open(ruta)
	if err != nil {
		return Config{}, fmt.Errorf("no se pudo abrir la configuración %q", ruta)
	}
	defer archivo.Close()
	return leerSistema(archivo)
}

func DatosUsuario() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir, _ = os.UserHomeDir()
	}
	return filepath.Join(dir, "monitor-camaras")
}

func DatosCentral() string {
	if runtime.GOOS == "windows" {
		return DatosUsuario()
	}
	return "/var/lib/monitor"
}

func Leer(r io.Reader) (Config, error) {
	cfg, err := leerSistema(r)
	if err != nil {
		return cfg, err
	}
	return cfg, ValidarSucursales(cfg.Sucursales)
}

func leerSistema(r io.Reader) (Config, error) {
	cfg := Config{
		Datos:    DatosCentral(),
		Servidor: Servidor{Escuchar: ":8080"},
		Go2rtc:   Go2rtc{Binario: "/usr/local/bin/go2rtc", API: "127.0.0.1:1984"},
		Interfaz: Interfaz{25, true, 60},
		Salud:    Salud{10, 3, 3},
	}
	dec := yaml.NewDecoder(r)
	if err := dec.Decode(&cfg); err != nil && err != io.EOF {
		return Config{}, fmt.Errorf("no se pudo leer la configuración YAML")
	}
	var adicional yaml.Node
	if err := dec.Decode(&adicional); err != io.EOF {
		return Config{}, fmt.Errorf("se esperaba un solo documento YAML")
	}
	if cfg.Datos == "" {
		cfg.Datos = DatosCentral()
	}
	if cfg.Go2rtc.Config == "" {
		cfg.Go2rtc.Config = filepath.Join(cfg.Datos, "go2rtc.yaml")
	}
	return cfg, cfg.ValidarSistema()
}

var idValido = regexp.MustCompile(`^[a-z0-9_]+$`)

func (d DVR) PuertoEfectivo() int {
	if d.Puerto != 0 {
		return d.Puerto
	}
	switch d.Marca {
	case "xmeye":
		return 34567
	case "onvif":
		return 80
	default:
		return 554
	}
}

func (d DVR) Direccion() string {
	return net.JoinHostPort(strings.Trim(d.Host, "[]"), strconv.Itoa(d.PuertoEfectivo()))
}

func (c Config) Validar() error {
	if err := c.ValidarSistema(); err != nil {
		return err
	}
	return ValidarSucursales(c.Sucursales)
}

func (c Config) ValidarSistema() error {
	hostAPI, puertoAPI, err := net.SplitHostPort(c.Go2rtc.API)
	puerto, errPuerto := strconv.Atoi(puertoAPI)
	if err != nil || errPuerto != nil || puerto < 1 || puerto > 65535 ||
		(!strings.EqualFold(hostAPI, "localhost") && !net.ParseIP(hostAPI).IsLoopback()) {
		return fmt.Errorf("go2rtc.api: se requiere una dirección local con puerto válido")
	}
	if c.Salud.IntervaloSegundos < 1 || int64(c.Salud.IntervaloSegundos) > math.MaxInt64/int64(time.Second) {
		return fmt.Errorf("salud.intervalo_segundos: fuera de rango")
	}
	if c.Salud.CaidoTrasFallos < 1 || c.Salud.CaidoTrasFallos > math.MaxInt/c.Salud.IntervaloSegundos {
		return fmt.Errorf("salud.caido_tras_fallos: fuera de rango")
	}
	if c.Salud.RecuperarTrasMinutos < 0 || int64(c.Salud.RecuperarTrasMinutos) > math.MaxInt64/int64(time.Minute) {
		return fmt.Errorf("salud.recuperar_tras_minutos: fuera de rango")
	}
	return nil
}

func ValidarSucursales(sucursales []Sucursal) error {
	ids := make(map[string]bool)
	for i, s := range sucursales {
		if !idValido.MatchString(s.ID) {
			return fmt.Errorf("sucursal %d, id: debe cumplir [a-z0-9_]+", i+1)
		}
		if ids[s.ID] {
			return fmt.Errorf("sucursal %q, id: repetido", s.ID)
		}
		ids[s.ID] = true
		switch s.DVR.Marca {
		case "hikvision", "dahua", "uniview", "xmeye", "onvif":
		case "rtsp":
			if strings.TrimSpace(s.DVR.RutaSub) == "" {
				return fmt.Errorf("sucursal %q, dvr.ruta_sub: obligatoria para rtsp", s.ID)
			}
		default:
			return fmt.Errorf("sucursal %q, dvr.marca: marca desconocida", s.ID)
		}
		if strings.TrimSpace(s.DVR.Host) == "" {
			return fmt.Errorf("sucursal %q, dvr.host: obligatorio", s.ID)
		}
		if s.DVR.Puerto < 0 || s.DVR.Puerto > 65535 {
			return fmt.Errorf("sucursal %q, dvr.puerto: fuera de rango", s.ID)
		}
		if len(s.Camaras) == 0 {
			return fmt.Errorf("sucursal %q, camaras: se requiere al menos una cámara", s.ID)
		}
		canales := make(map[int]bool)
		for _, cam := range s.Camaras {
			if cam.Canal < 1 {
				return fmt.Errorf("sucursal %q, camaras.canal: debe ser mayor o igual a 1", s.ID)
			}
			if canales[cam.Canal] {
				return fmt.Errorf("sucursal %q, camaras.canal: canal %d repetido", s.ID, cam.Canal)
			}
			canales[cam.Canal] = true
		}
	}
	return nil
}
