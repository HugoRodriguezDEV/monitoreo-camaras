package almacen

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/HugoRodriguezDEV/monitoreo-camaras/internal/config"
)

var (
	ErrAusente   = errors.New("no se encontró el registro")
	ErrExiste    = errors.New("el registro ya existe")
	ErrConflicto = errors.New("el registro cambió mientras se realizaba la operación")
	ErrGuardar   = errors.New("no se pudo guardar en el directorio de datos")
)

type Ajustes struct {
	Interfaz config.Interfaz `json:"interfaz"`
	Alertas  config.Alertas  `json:"alertas"`
}

type Vinculo struct {
	ID        string     `json:"id"`
	Nombre    string     `json:"nombre"`
	TokenHash string     `json:"tokenHash"`
	Creado    time.Time  `json:"creado"`
	UltimoUso *time.Time `json:"ultimoUso"`
}

type Admin struct {
	PasswordHash string    `json:"passwordHash"`
	Vinculos     []Vinculo `json:"vinculos"`
}

type Almacen struct {
	dir     string
	mu      sync.RWMutex
	cfg     config.Config
	admin   Admin
	version uint64
	subs    map[chan struct{}]struct{}
}

func Abrir(cfg config.Config) (*Almacen, error) {
	if cfg.Datos == "" {
		cfg.Datos = config.DatosCentral()
	}
	if err := os.MkdirAll(cfg.Datos, 0700); err != nil {
		return nil, ErrGuardar
	}
	a := &Almacen{dir: cfg.Datos, cfg: cfg, version: 1, subs: make(map[chan struct{}]struct{})}
	ajustes := Ajustes{cfg.Interfaz, cfg.Alertas}
	a.admin.Vinculos = []Vinculo{}
	sinSucursales := false
	if err := Leer(a.dir, "sucursales.json", &a.cfg.Sucursales); errors.Is(err, os.ErrNotExist) {
		sinSucursales = true
	} else if err != nil {
		return nil, err
	}
	if err := a.cfg.Validar(); err != nil {
		return nil, err
	}
	if a.cfg.Sucursales == nil {
		a.cfg.Sucursales = []config.Sucursal{}
	}
	if err := abrirOCrear(a.dir, "ajustes.json", &ajustes); err != nil {
		return nil, err
	}
	if err := abrirOCrear(a.dir, "admin.json", &a.admin); err != nil {
		return nil, err
	}
	if sinSucursales {
		if err := Guardar(a.dir, "sucursales.json", a.cfg.Sucursales); err != nil {
			return nil, err
		}
	}
	a.cfg.Interfaz, a.cfg.Alertas = ajustes.Interfaz, ajustes.Alertas
	a.cfg.Sucursales = clonarSucursales(a.cfg.Sucursales)
	return a, nil
}

func abrirOCrear(dir, nombre string, valor any) error {
	err := Leer(dir, nombre, valor)
	if errors.Is(err, os.ErrNotExist) {
		return Guardar(dir, nombre, valor)
	}
	return err
}

func Leer(dir, nombre string, valor any) error {
	datos, err := leerArchivo(filepath.Join(dir, nombre))
	if errors.Is(err, os.ErrNotExist) {
		return os.ErrNotExist
	}
	if err != nil {
		return fmt.Errorf("no se pudo leer %s", nombre)
	}
	if err := json.Unmarshal(datos, valor); err != nil {
		return fmt.Errorf("%s contiene JSON inválido", nombre)
	}
	return nil
}

func Guardar(dir, nombre string, valor any) error {
	datos, err := json.MarshalIndent(valor, "", "  ")
	if err != nil {
		return ErrGuardar
	}
	archivo, err := os.CreateTemp(dir, ".monitor-*")
	if err != nil {
		return ErrGuardar
	}
	defer os.Remove(archivo.Name())
	_, err = archivo.Write(append(datos, '\n'))
	if err == nil {
		err = archivo.Sync()
	}
	cierre := archivo.Close()
	if err != nil || cierre != nil {
		return ErrGuardar
	}
	if err := reemplazar(archivo.Name(), filepath.Join(dir, nombre)); err != nil {
		return ErrGuardar
	}
	return nil
}

func clonarSucursales(lista []config.Sucursal) []config.Sucursal {
	copia := slices.Clone(lista)
	for i := range copia {
		copia[i].Camaras = slices.Clone(copia[i].Camaras)
		if copia[i].Huella != nil {
			huella := *copia[i].Huella
			copia[i].Huella = &huella
		}
	}
	return copia
}

func (a *Almacen) Actual() (config.Config, uint64) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	cfg := a.cfg
	cfg.Sucursales = clonarSucursales(cfg.Sucursales)
	return cfg, a.version
}

// Suscribir notifica que hay una versión más reciente. Actual obtiene la última.
func (a *Almacen) Suscribir() (<-chan struct{}, func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	ch := make(chan struct{}, 1)
	a.subs[ch] = struct{}{}
	return ch, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if _, existe := a.subs[ch]; existe {
			delete(a.subs, ch)
			close(ch)
		}
	}
}

func (a *Almacen) notificar() {
	a.version++
	for ch := range a.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (a *Almacen) EditarSucursales(editar func([]config.Sucursal) ([]config.Sucursal, error)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	lista, err := editar(clonarSucursales(a.cfg.Sucursales))
	if err != nil {
		return err
	}
	if err := config.ValidarSucursales(lista); err != nil {
		return err
	}
	if err := Guardar(a.dir, "sucursales.json", lista); err != nil {
		return err
	}
	a.cfg.Sucursales = clonarSucursales(lista)
	a.notificar()
	return nil
}

func (a *Almacen) EditarAjustes(editar func(*Ajustes)) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	ajustes := Ajustes{a.cfg.Interfaz, a.cfg.Alertas}
	editar(&ajustes)
	if err := Guardar(a.dir, "ajustes.json", ajustes); err != nil {
		return err
	}
	a.cfg.Interfaz, a.cfg.Alertas = ajustes.Interfaz, ajustes.Alertas
	a.notificar()
	return nil
}

func clonarAdmin(admin Admin) Admin {
	admin.Vinculos = slices.Clone(admin.Vinculos)
	for i := range admin.Vinculos {
		if admin.Vinculos[i].UltimoUso != nil {
			ultimoUso := *admin.Vinculos[i].UltimoUso
			admin.Vinculos[i].UltimoUso = &ultimoUso
		}
	}
	return admin
}

func (a *Almacen) Admin() Admin {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return clonarAdmin(a.admin)
}

func (a *Almacen) EditarAdmin(editar func(*Admin) error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	admin := clonarAdmin(a.admin)
	if err := editar(&admin); err != nil {
		return err
	}
	if err := Guardar(a.dir, "admin.json", admin); err != nil {
		return err
	}
	a.admin = clonarAdmin(admin)
	return nil
}

func Indice(lista []config.Sucursal, id string) int {
	return slices.IndexFunc(lista, func(s config.Sucursal) bool { return s.ID == id })
}
