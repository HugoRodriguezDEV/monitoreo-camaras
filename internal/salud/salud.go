package salud

import (
	"context"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

const TiempoLimite = 3 * time.Second

type Sucursal struct {
	ID         string    `json:"id"`
	EnLinea    bool      `json:"enLinea"`
	Desde      time.Time `json:"desde"`
	LatenciaMS int64     `json:"latenciaMs"`
	Error      string    `json:"error"`
}

type Go2rtc struct {
	EnLinea bool `json:"enLinea"`
}

type Estado struct {
	ConfigVersion uint64     `json:"configVersion"`
	Actualizado   time.Time  `json:"actualizado"`
	Go2rtc        Go2rtc     `json:"go2rtc"`
	Sucursales    []Sucursal `json:"sucursales"`
}

type Cambio struct {
	Nombre        string
	EnLinea       bool
	DuracionCaida time.Duration
}

type Sondas struct {
	TCP func(context.Context, string) (time.Duration, error)
	API func(context.Context, string) bool
}

type Monitor struct {
	cfg       config.Config
	sondas    Sondas
	alCambiar func(Cambio)
	mu        sync.RWMutex
	estado    Estado
	fallos    []int
	revision  uint64
	sondeo    sync.Mutex
}

func Nuevo(cfg config.Config, sondas Sondas, alCambiar func(Cambio)) *Monitor {
	if sondas.TCP == nil {
		sondas.TCP = SondearTCP
	}
	if sondas.API == nil {
		sondas.API = sondearAPI
	}
	ahora := time.Now().UTC()
	estado := Estado{Actualizado: ahora, Sucursales: make([]Sucursal, len(cfg.Sucursales))}
	for i, s := range cfg.Sucursales {
		estado.Sucursales[i] = Sucursal{ID: s.ID, EnLinea: true, Desde: ahora}
	}
	cfg.Sucursales = slices.Clone(cfg.Sucursales)
	return &Monitor{cfg: cfg, sondas: sondas, alCambiar: alCambiar, estado: estado, fallos: make([]int, len(cfg.Sucursales))}
}

func SondearTCP(ctx context.Context, direccion string) (time.Duration, error) {
	inicio := time.Now()
	dialer := net.Dialer{Timeout: TiempoLimite}
	conn, err := dialer.DialContext(ctx, "tcp", direccion)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	return time.Since(inicio), nil
}

func sondearAPI(ctx context.Context, direccion string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+direccion+"/api", nil)
	if err != nil {
		return false
	}
	cliente := http.Client{
		Timeout:       TiempoLimite,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (m *Monitor) Estado() Estado {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copia := m.estado
	copia.Sucursales = append([]Sucursal{}, copia.Sucursales...)
	return copia
}

func (m *Monitor) Ejecutar(ctx context.Context) {
	reloj := time.NewTicker(time.Duration(m.cfg.Salud.IntervaloSegundos) * time.Second)
	defer reloj.Stop()
	for {
		m.Sondear(ctx)
		select {
		case <-ctx.Done():
			return
		case <-reloj.C:
		}
	}
}

// Actualizar conserva la historia solo cuando el destino del sondeo sigue siendo el mismo.
func (m *Monitor) Actualizar(sucursales []config.Sucursal, version uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	indices := make(map[string]int, len(m.cfg.Sucursales))
	for i, s := range m.cfg.Sucursales {
		indices[s.ID] = i
	}
	estados := make([]Sucursal, len(sucursales))
	fallos := make([]int, len(sucursales))
	ahora := time.Now().UTC()
	for i, s := range sucursales {
		if anterior, ok := indices[s.ID]; ok && m.cfg.Sucursales[anterior].DVR == s.DVR {
			estados[i], fallos[i] = m.estado.Sucursales[anterior], m.fallos[anterior]
		} else {
			estados[i] = Sucursal{ID: s.ID, EnLinea: true, Desde: ahora}
		}
	}
	m.cfg.Sucursales = slices.Clone(sucursales)
	m.estado.Sucursales, m.fallos = estados, fallos
	m.estado.ConfigVersion = version
	m.revision++
}

func (m *Monitor) Sondear(ctx context.Context) {
	m.sondeo.Lock()
	defer m.sondeo.Unlock()
	m.mu.RLock()
	sucursales, revision := m.cfg.Sucursales, m.revision
	m.mu.RUnlock()
	type resultado struct {
		latencia time.Duration
		err      error
	}
	resultados := make([]resultado, len(sucursales))
	var apiEnLinea bool
	var grupo sync.WaitGroup
	for i, suc := range sucursales {
		grupo.Go(func() {
			latencia, err := m.sondas.TCP(ctx, suc.DVR.Direccion())
			resultados[i] = resultado{latencia, err}
		})
	}
	grupo.Go(func() { apiEnLinea = m.sondas.API(ctx, m.cfg.Go2rtc.API) })
	grupo.Wait()
	if ctx.Err() != nil {
		return
	}

	ahora := time.Now().UTC()
	var cambios []Cambio
	m.mu.Lock()
	if m.revision != revision {
		m.mu.Unlock()
		return
	}
	m.estado.Actualizado = ahora
	m.estado.Go2rtc.EnLinea = apiEnLinea
	for i, r := range resultados {
		s := &m.estado.Sucursales[i]
		enLinea := s.EnLinea
		if r.err == nil {
			m.fallos[i] = 0
			enLinea = true
			s.Error = ""
			s.LatenciaMS = r.latencia.Milliseconds()
		} else {
			if m.fallos[i] < m.cfg.Salud.CaidoTrasFallos {
				m.fallos[i]++
			}
			enLinea = m.fallos[i] < m.cfg.Salud.CaidoTrasFallos
			s.Error = "No se pudo establecer la conexión TCP"
			s.LatenciaMS = 0
		}
		if enLinea != s.EnLinea {
			cambio := Cambio{Nombre: m.cfg.Sucursales[i].Nombre, EnLinea: enLinea}
			if enLinea {
				cambio.DuracionCaida = ahora.Sub(s.Desde)
			}
			s.EnLinea = enLinea
			s.Desde = ahora
			cambios = append(cambios, cambio)
		}
	}
	m.mu.Unlock()
	if m.alCambiar != nil {
		for _, cambio := range cambios {
			m.alCambiar(cambio)
		}
	}
}
