package salud

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

func TestHisteresis(t *testing.T) {
	cfg := config.Config{
		Salud:      config.Salud{IntervaloSegundos: 10, CaidoTrasFallos: 3},
		Sucursales: []config.Sucursal{{ID: "s1", Nombre: "Centro"}},
	}
	caido := false
	var cambios []Cambio
	m := Nuevo(cfg, Sondas{
		TCP: func(context.Context, string) (time.Duration, error) {
			if caido {
				return 0, errors.New("fallo simulado")
			}
			return 42 * time.Millisecond, nil
		},
		API: func(context.Context, string) bool { return !caido },
	}, func(c Cambio) { cambios = append(cambios, c) })
	pasos := []struct {
		falla, enLinea bool
		cambios        int
	}{
		{false, true, 0}, {true, true, 0}, {true, true, 0}, {false, true, 0},
		{true, true, 0}, {true, true, 0}, {true, false, 1}, {true, false, 1}, {false, true, 2},
	}
	for i, p := range pasos {
		if i == len(pasos)-1 {
			m.estado.Sucursales[0].Desde = time.Now().UTC().Add(-12 * time.Minute)
		}
		caido = p.falla
		m.Sondear(context.Background())
		estado := m.Estado()
		s := estado.Sucursales[0]
		if s.EnLinea != p.enLinea || len(cambios) != p.cambios || estado.Go2rtc.EnLinea == p.falla {
			t.Fatalf("Sondeo %d: estado %+v, cambios %+v", i+1, estado, cambios)
		}
		if p.falla && (s.Error == "" || s.LatenciaMS != 0) || !p.falla && (s.Error != "" || s.LatenciaMS != 42) {
			t.Fatalf("Sondeo %d: latencia o error incorrectos: %+v", i+1, s)
		}
	}
	if cambios[0].EnLinea || !cambios[1].EnLinea || cambios[1].Nombre != "Centro" || cambios[1].DuracionCaida.Round(time.Minute) != 12*time.Minute {
		t.Fatalf("Cambios incorrectos: %+v", cambios)
	}
}
