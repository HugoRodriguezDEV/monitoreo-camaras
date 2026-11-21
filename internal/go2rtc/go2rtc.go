package go2rtc

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/marcas"
	"gopkg.in/yaml.v3"
)

func Generar(cfg config.Config) ([]byte, error) {
	type api struct {
		Listen string
		Origin string
	}
	documento := struct {
		API     api
		Streams map[string]string
	}{API: api{Listen: cfg.Go2rtc.API}, Streams: make(map[string]string)}
	for _, suc := range cfg.Sucursales {
		for _, cam := range suc.Camaras {
			for _, hd := range []bool{false, true} {
				documento.Streams[marcas.Stream(suc.ID, cam.Canal, hd)] = marcas.URL(suc.DVR, cam.Canal, hd)
			}
		}
	}
	return yaml.Marshal(documento)
}

func Preparar(cfg config.Config) error {
	if cfg.Go2rtc.Externo {
		return nil
	}
	contenido, err := Generar(cfg)
	if err != nil {
		return fmt.Errorf("no se pudo generar la configuración de go2rtc")
	}

	ruta := cfg.Go2rtc.Config
	temporal, err := os.CreateTemp(filepath.Dir(ruta), ".go2rtc-*.yaml")
	if err != nil {
		return fmt.Errorf("no se pudo crear un archivo temporal para %q", ruta)
	}
	defer os.Remove(temporal.Name())
	_, errEscritura := temporal.Write(contenido)
	errCierre := temporal.Close()
	if errEscritura != nil || errCierre != nil {
		return fmt.Errorf("no se pudo guardar la configuración de go2rtc")
	}
	if err := os.Rename(temporal.Name(), ruta); err != nil {
		return fmt.Errorf("no se pudo reemplazar %q", ruta)
	}
	return nil
}

type salida struct{ pendiente []byte }

func (s *salida) Write(p []byte) (int, error) {
	n := len(p)
	for {
		linea, resto, completa := bytes.Cut(p, []byte{'\n'})
		s.pendiente = append(s.pendiente, linea...)
		if !completa {
			return n, nil
		}
		s.registrar()
		p = resto
	}
}

func (s *salida) registrar() {
	if len(s.pendiente) == 0 {
		return
	}
	log.Printf("[go2rtc] %s", bytes.TrimSuffix(s.pendiente, []byte{'\r'}))
	s.pendiente = s.pendiente[:0]
}

func Supervisar(ctx context.Context, cfg config.Go2rtc) {
	if cfg.Externo {
		return
	}
	espera := time.Second
	for ctx.Err() == nil {
		inicio := time.Now()
		cmd := exec.CommandContext(ctx, cfg.Binario, "-config", cfg.Config)
		cmd.Cancel = func() error { return detener(cmd.Process) }
		cmd.WaitDelay = 5 * time.Second
		registro := &salida{}
		cmd.Stdout, cmd.Stderr = registro, registro
		err := cmd.Run()
		registro.registrar()
		if ctx.Err() != nil {
			return
		}
		if time.Since(inicio) >= time.Minute {
			espera = time.Second
		}
		switch {
		case cmd.Process == nil:
			log.Printf("[go2rtc] No se pudo iniciar %q", cfg.Binario)
		case err != nil:
			log.Printf("[go2rtc] El proceso terminó con código %d", cmd.ProcessState.ExitCode())
		default:
			log.Print("[go2rtc] El proceso terminó")
		}
		log.Printf("[go2rtc] Reinicio en %s", espera)
		temporizador := time.NewTimer(espera)
		select {
		case <-ctx.Done():
			temporizador.Stop()
			return
		case <-temporizador.C:
		}
		espera = min(espera*2, 30*time.Second)
	}
}
