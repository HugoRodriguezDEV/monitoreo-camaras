package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/cmd/dvrfalso/simulador"
)

func main() {
	var op simulador.Opciones
	escuchar := flag.String("escuchar", "127.0.0.1:8081", "dirección del servidor")
	flag.StringVar(&op.Marca, "marca", "hikvision", "marca: hikvision, dahua u onvif")
	flag.StringVar(&op.Usuario, "usuario", "admin", "usuario del DVR")
	flag.StringVar(&op.Password, "password", "admin123", "contraseña del DVR")
	flag.IntVar(&op.Canales, "canales", 4, "cantidad de canales")
	flag.StringVar(&op.MAC, "mac", "02:11:22:33:44:55", "dirección MAC simulada")
	flag.StringVar(&op.Serie, "serie", "SIMULADO-001", "número de serie simulado")
	h265 := flag.String("h265", "3", "canales H.265 separados por comas")
	flag.BoolVar(&op.DHCP, "dhcp", false, "simular una dirección IP asignada por el módem")
	flag.Parse()
	if op.Canales < 1 {
		log.Fatal("se requiere al menos un canal")
	}
	op.H265 = map[int]bool{}
	if *h265 != "" {
		for _, s := range strings.Split(*h265, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || n < 1 {
				log.Fatalf("canal H.265 inválido: %q", s)
			}
			op.H265[n] = true
		}
	}
	switch op.Marca {
	case "hikvision", "dahua", "onvif":
	default:
		log.Fatalf("marca inválida: %q", op.Marca)
	}
	servidor := http.Server{Addr: *escuchar, Handler: simulador.Nuevo(op), ReadHeaderTimeout: 5 * time.Second}
	log.Printf("DVR simulado %s en %s", op.Marca, *escuchar)
	log.Fatal(servidor.ListenAndServe())
}
