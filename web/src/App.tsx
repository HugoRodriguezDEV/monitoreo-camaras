import { useCallback, useEffect, useMemo, useState } from 'react'
import { CameraTile } from './components/CameraTile'
import { Grid, type Celda } from './components/Grid'
import { Reloj } from './components/Reloj'
import { cargarConfig, cargarReproductor, type Config } from './config'
import { useEstado } from './useEstado'

/** 'general' o el id de una sucursal. */
type Vista = string
const GENERAL = 'general'

export default function App() {
  const [config, setConfig] = useState<Config>()
  const [error, setError] = useState<string>()

  useEffect(() => {
    cargarConfig()
      .then(setConfig)
      .catch((e: Error) => setError(e.message))
  }, [])

  if (error) return <div className="pantalla-mensaje error">{error}</div>
  if (!config) return <div className="pantalla-mensaje">Cargando…</div>
  return <Monitor config={config} />
}

function Monitor({ config }: { config: Config }) {
  const estado = useEstado()
  const caidas = new Set(
    estado === 'desconocido' ? [] : estado.sucursales.filter((s) => s.enLinea === false).map((s) => s.id),
  )
  const servidorCaido = estado !== 'desconocido' && estado.go2rtc.enLinea === false
  const [reproductorListo, setReproductorListo] = useState(false)
  useEffect(() => {
    let desmontado = false
    let reintento: ReturnType<typeof setTimeout>

    function cargar() {
      cargarReproductor(config)
        .then(() => {
          if (!desmontado) setReproductorListo(true)
        })
        .catch(() => {
          if (!desmontado) reintento = setTimeout(cargar, 5000)
        })
    }

    cargar()
    return () => {
      desmontado = true
      clearTimeout(reintento)
    }
  }, [config])

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
  const avisos = config.sucursales.filter(
    (s) => caidas.has(s.id) && (enGrande ? s.id === enGrande.sucursalId : vista === GENERAL || s.id === vista),
  )

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
              {caidas.has(s.id) && <span className="punto-caida" role="img" aria-label="Sin conexión" />}
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
        {!reproductorListo ? (
          <div className="pantalla-mensaje">Conectando con el servidor de video…</div>
        ) : enGrande ? (
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
          <Grid
            key={vista}
            config={config}
            celdas={celdas}
            onSeleccionar={setEnGrande}
            detectarPorVideo={estado === 'desconocido'}
            sucursalesCaidas={vista === GENERAL ? caidas : undefined}
          />
        )}
        {(servidorCaido || avisos.length > 0) && (
          <div className="aviso-desconexion" role="status">
            {servidorCaido && <div>Servidor de video sin conexión</div>}
            {avisos.map((s) => (
              <div key={s.id}>
                {s.nombre} sin conexión <span className="reintentando">· reintentando</span>
              </div>
            ))}
          </div>
        )}
      </main>
    </div>
  )
}
