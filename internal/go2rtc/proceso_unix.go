//go:build !windows

package go2rtc

import (
	"os"
	"syscall"
)

func detener(proceso *os.Process) error { return proceso.Signal(syscall.SIGTERM) }
