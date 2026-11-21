import { useCallback, useEffect, useMemo, useState } from 'react'
import { CameraTile } from './components/CameraTile'
import { Grid, type Celda } from './components/Grid'
import { Reloj } from './components/Reloj'
import { cargarConfig, cargarReproductor, type Config } from './config'

/** 'general' o el id de una sucursal. */
type Vista = string
const GENERAL = 'general'

export default function App() {
  const [config, setConfig] = useState<Config>()
  const [error, setError] = useState<string>()

  useEffect(() => {
    cargarConfig()
      .then(async (c) => {
        await cargarReproductor(c)
        setConfig(c)
      })
      .catch((e: Error) => setError(e.message))
  }, [])

  if (error) return <div className="pantalla-mensaje error">{error}</div>
  if (!config) return <div className="pantalla-mensaje">Cargando…</div>
  return <Monitor config={config} />
}

function Monitor({ config }: { config: Config }) {
  const secuencia = useMemo<Vista[]>(
    () => [...(config.rotarVistaGeneral ? [GENERAL] : []), ...config.sucursales.map((s) => s.id)],
    [config],
  )
  const [vista, setVista] = useState<Vista>(secuencia[0])
  const [rotando, setRotando] = useState(true)
  const [enGrande, setEnGrande] = useState<Celda>()

  // Rotación automática; se detiene mientras hay una cámara en grande.
  useEffect(() => {
    if (!rotando || enGrande || secuencia.length < 2) return
    const t = setTimeout(() => {
      setVista((v) => secuencia[(secuencia.indexOf(v) + 1) % secuencia.length])
    }, config.rotacionSegundos * 1000)
    return () => clearTimeout(t)
  }, [vista, rotando, enGrande, secuencia, config.rotacionSegundos])

  // La cámara en grande vuelve sola a la cuadrícula.
  useEffect(() => {
    if (!enGrande) return
    const t = setTimeout(() => setEnGrande(undefined), config.volverACuadriculaSegundos * 1000)
    return () => clearTimeout(t)
  }, [enGrande, config.volverACuadriculaSegundos])

  const celdas = useMemo<Celda[]>(() => {
    if (vista === GENERAL) {
      return config.sucursales.flatMap((s) => {
        const claves = s.camaras.filter((c) => c.clave)
        return (claves.length ? claves : s.camaras.slice(0, 1)).map((camara) => ({
          camara,
          sucursal: s.nombre,
          sucursalId: s.id,
        }))
      })
    }
    const s = config.sucursales.find((x) => x.id === vista)
    return s ? s.camaras.map((camara) => ({ camara, sucursal: s.nombre, sucursalId: s.id })) : []
  }, [config, vista])

  const titulo = vista === GENERAL ? 'Vista general' : config.sucursales.find((s) => s.id === vista)?.nombre

  const irA = useCallback((v: Vista) => {
    setEnGrande(undefined)
    setVista(v)
  }, [])

  return (
    <div className="monitor">
      <header className="barra">
        <span className="titulo">{enGrande ? `${enGrande.sucursal} · ${enGrande.camara.nombre}` : titulo}</span>
        <nav className="vistas">
          {config.rotarVistaGeneral && (
            <button className={vista === GENERAL ? 'activo' : ''} onClick={() => irA(GENERAL)}>
              General
            </button>
          )}
          {config.sucursales.map((s) => (
            <button key={s.id} className={vista === s.id ? 'activo' : ''} onClick={() => irA(s.id)}>
              {s.nombre}
            </button>
          ))}
          <button className={rotando ? 'rotando' : ''} onClick={() => setRotando((r) => !r)}>
            {rotando ? '⟳ Rotando' : '⏸ Fijo'}
          </button>
        </nav>
        <Reloj />
      </header>

      <main className="contenido">
        {enGrande ? (
          <div className="en-grande" onClick={() => setEnGrande(undefined)}>
            <CameraTile
              config={config}
              stream={enGrande.camara.streamHd ?? enGrande.camara.stream}
              nombre={enGrande.camara.nombre}
              sucursal={enGrande.sucursal}
            />
          </div>
        ) : (
          // key: cada vista arranca de cero el conteo de "sin conexión".
          <Grid key={vista} config={config} celdas={celdas} onSeleccionar={setEnGrande} />
        )}
      </main>
    </div>
  )
}
