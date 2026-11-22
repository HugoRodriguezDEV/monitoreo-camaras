//go:build !windows

package go2rtc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

func TestSupervisionReiniciaElProceso(t *testing.T) {
	dir := t.TempDir()
	ejecuciones := filepath.Join(dir, "ejecuciones")
	binario := filepath.Join(dir, "go2rtc-falso")
	t.Setenv("MONITOR_PRUEBA_GO2RTC", ejecuciones)
	if err := os.WriteFile(binario, []byte(`#!/bin/sh
echo "$$" >> "$MONITOR_PRUEBA_GO2RTC"
exec sleep 60
`), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, fin := context.WithCancel(context.Background())
	reinicio := make(chan struct{}, 1)
	listo := make(chan struct{})
	go func() {
		defer close(listo)
		SupervisarConReinicio(ctx, config.Go2rtc{Binario: binario}, reinicio)
	}()
	defer func() { fin(); <-listo }()
	esperar := func(veces int) []string {
		t.Helper()
		limite := time.Now().Add(5 * time.Second)
		for time.Now().Before(limite) {
			b, _ := os.ReadFile(ejecuciones)
			pids := strings.Fields(string(b))
			if len(pids) >= veces {
				return pids
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("no arrancó el proceso %d", veces)
		return nil
	}
	esperar(1)
	reinicio <- struct{}{}
	pids := esperar(2)
	if pids[0] == pids[1] {
		t.Fatal("el proceso no cambió después del reinicio")
	}
}
