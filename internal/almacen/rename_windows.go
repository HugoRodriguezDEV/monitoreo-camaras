package almacen

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func reintentarArchivo(operacion func() error) error {
	// NTFS puede negar el acceso brevemente mientras se publica el reemplazo.
	limite := time.Now().Add(250 * time.Millisecond)
	for {
		err := operacion()
		if (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) || time.Now().After(limite) {
			return err
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func leerArchivo(ruta string) ([]byte, error) {
	var datos []byte
	err := reintentarArchivo(func() error {
		var err error
		datos, err = os.ReadFile(ruta)
		return err
	})
	return datos, err
}

func reemplazar(origen, destino string) error {
	desde, err := windows.UTF16PtrFromString(origen)
	if err != nil {
		return err
	}
	hasta, err := windows.UTF16PtrFromString(destino)
	if err != nil {
		return err
	}
	return reintentarArchivo(func() error {
		return windows.MoveFileEx(desde, hasta, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
	})
}
