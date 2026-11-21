package go2rtc

import "os"

func detener(proceso *os.Process) error { return proceso.Kill() }
