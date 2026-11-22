//go:build !windows

package almacen

import "os"

var reemplazar = os.Rename
var leerArchivo = os.ReadFile
