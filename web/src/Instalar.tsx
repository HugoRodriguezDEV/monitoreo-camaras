import { useEffect, useState, type FormEvent } from 'react'
import { api, dvrNuevo, instalador, respuesta, solicitud, type Camara, type Dvr, type Identificacion, type Sucursal } from './api'
import { Advertencias, CamposCamara, CamposDvr, CamposSucursal, Marco, Mensajes } from './Formularios'

type Central = { url: string; vinculado: boolean; nombre: string }
type Hallazgo = { ip: string; marca: string; modelo: string; pista: string }
type Barrido = { terminado: boolean; hechos: number; total: number; hallazgos: Hallazgo[] }
type SucursalCentral = Pick<Sucursal, 'id' | 'nombre'> & { dvr: Pick<Dvr, 'host'> }
const pasos = ['Central', 'Buscar', 'Acceso', 'Revisión y vista previa', 'Enviar']

function rango(host: string) {
  const partes = host.split('.')
  return partes.length === 4 && partes.every(p => /^\d{1,3}$/.test(p) && Number(p) <= 255)
    ? partes.slice(0, 3).map(Number).join('.') : undefined
}

export default function Instalar() {
  const [paso, setPaso] = useState(0)
  const [central, setCentral] = useState<Central>()
  const [url, setUrl] = useState('')
  const [token, setToken] = useState('')
  const [soloExportar, setSoloExportar] = useState(false)
  const [redes, setRedes] = useState<string[]>([])
  const [busqueda, setBusqueda] = useState('')
  const [barrido, setBarrido] = useState<Barrido>()
  const [dvr, setDvr] = useState<Dvr>(dvrNuevo)
  const [identificacion, setIdentificacion] = useState<Identificacion>()
  const [camaras, setCamaras] = useState<Camara[]>([])
  const [sucursal, setSucursal] = useState({ id: '', nombre: '' })
  const [existentes, setExistentes] = useState<SucursalCentral[]>([])
  const [aviso, setAviso] = useState('')
  const [exito, setExito] = useState('')
  const [error, setError] = useState('')
  const [ocupado, setOcupado] = useState(false)

  useEffect(() => {
    let activo = true
    void api<Central>(instalador + '/central').then(datos => {
      if (!activo) return
      setCentral(datos)
      setUrl(actual => actual || datos.url)
    }).catch((e: Error) => { if (activo) setError(e.message) })
    return () => { activo = false }
  }, [])

  useEffect(() => {
    if (paso !== 1) return
    let activo = true
    void api<{ redes: string[] }>(instalador + '/redes').then(datos => {
      if (activo) setRedes(datos.redes ?? [])
    }).catch((e: Error) => { if (activo) setError(e.message) })
    return () => { activo = false }
  }, [paso])

  useEffect(() => {
    if (paso !== 1 || !busqueda) return
    const control = new AbortController()
    let siguiente: ReturnType<typeof setTimeout>
    async function consultar() {
      try {
        const progreso = await api<Barrido>(instalador + '/buscar/' + encodeURIComponent(busqueda), { signal: control.signal })
        if (control.signal.aborted) return
        setBarrido(progreso)
        if (progreso.terminado) setBusqueda('')
        else siguiente = setTimeout(consultar, 500)
      } catch (e) {
        if (control.signal.aborted) return
        setError((e as Error).message)
        setBusqueda('')
      }
    }
    void consultar()
    return () => { control.abort(); clearTimeout(siguiente) }
  }, [busqueda, paso])

  useEffect(() => {
    if (paso !== 4 || soloExportar || exito) return
    const control = new AbortController()
    void api<SucursalCentral[]>(instalador + '/sucursales-central', { signal: control.signal }).then(datos => {
      if (!control.signal.aborted) setExistentes(datos ?? [])
    }).catch((e: Error) => {
      if (!control.signal.aborted) setAviso(e.message)
    })
    return () => control.abort()
  }, [paso, soloExportar, exito])

  function ir(n: number) {
    setError('')
    setAviso('')
    setPaso(n)
    window.scrollTo(0, 0)
  }

  async function ejecutar(accion: () => Promise<void>) {
    setOcupado(true)
    setError('')
    try {
      await accion()
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setOcupado(false)
    }
  }

  function vincular(e: FormEvent) {
    e.preventDefault()
    void ejecutar(async () => {
      if (token || !central?.vinculado || url !== central.url) {
        await api(instalador + '/central', solicitud('PUT', { url, token }))
        setCentral(await api<Central>(instalador + '/central'))
        setToken('')
      }
      setSoloExportar(false)
      ir(1)
    })
  }

  function buscar() {
    void ejecutar(async () => {
      setBarrido(undefined)
      const { id } = await api<{ id: string }>(instalador + '/buscar', solicitud('POST', { redes }))
      setBusqueda(id)
    })
  }

  function seleccionar(h?: Hallazgo) {
    setDvr({ ...dvrNuevo, host: h?.ip ?? '', marca: h?.marca || 'hikvision' })
    ir(2)
  }

  function identificar(e: FormEvent) {
    e.preventDefault()
    void ejecutar(async () => {
      const datos = await api<Identificacion>(instalador + '/identificar', solicitud('POST', dvr))
      setIdentificacion(datos)
      const canales = datos.identidad.canales ?? []
      setCamaras(canales.length ? canales.map((c, i) => ({
        canal: c.canal, nombre: c.nombre || `Cámara ${c.canal}`, clave: i === 0,
      })) : [{ canal: 1, nombre: 'Cámara 1', clave: true }])
      ir(3)
    })
  }

  function finalizar(accion: 'enviar' | 'exportar') {
    void ejecutar(async () => {
      const { marca, modelo, serie, mac } = identificacion?.identidad ?? {}
      const datos: Sucursal = { ...sucursal, dvr, camaras, huella: { marca, modelo, serie, mac, actualizada: new Date().toISOString() } }
      const opciones = solicitud('POST', datos)
      if (accion === 'exportar') {
        const archivo = await (await respuesta(instalador + '/exportar', opciones)).blob()
        const enlace = document.createElement('a')
        const descarga = URL.createObjectURL(archivo)
        enlace.href = descarga
        enlace.download = `sucursal-${sucursal.id}.json`
        enlace.click()
        setTimeout(() => URL.revokeObjectURL(descarga), 1000)
        setExito('Archivo listo para guardar. Contiene la contraseña del DVR; guárdalo en un lugar seguro.')
      } else {
        const resultado = await api<{ ok: boolean }>(instalador + '/enviar', opciones)
        if (!resultado?.ok) throw new Error('La central no confirmó que guardó la sucursal.')
        setExito('La central confirmó que guardó la sucursal.')
      }
      setDvr(dvrNuevo)
      window.scrollTo(0, 0)
    })
  }

  const redDvr = rango(dvr.host)
  const repetidas = soloExportar || !redDvr ? [] : existentes.filter(s => s.id !== sucursal.id && rango(s.dvr.host) === redDvr)
  const reemplazo = soloExportar ? undefined : existentes.find(s => s.id === sucursal.id)
  const sinCanales = !identificacion?.identidad.canales?.length

  return <Marco titulo="Asistente de instalación">
    {exito ? <section className="tarjeta pila">
      <h2>Instalación completada</h2><Mensajes aviso={exito} /><p>{sucursal.nombre}</p>
      <a href="/instalar">Instalar otra sucursal</a>
    </section> : <div className="pila">
      <ol className="pasos" aria-label="Pasos de instalación">
        {pasos.map((nombre, i) => <li key={nombre} aria-current={i === paso ? 'step' : undefined}>{nombre}</li>)}
      </ol>
      <section className="tarjeta pila">
        <h2>{paso + 1}. {pasos[paso]}</h2>
        <Mensajes error={error} />
        {paso === 0 && <form className="pila" onSubmit={vincular}>
          <p>La central es la computadora que muestra las cámaras. El panel de esa computadora genera el token en Vínculos.</p>
          {central?.vinculado && <Mensajes aviso={`Central vinculada: ${central.nombre || central.url}`} />}
          <label>Dirección de la central<input type="url" required placeholder="http://central:8080" value={url} onChange={e => setUrl(e.target.value)} /></label>
          <label>Token de vínculo<input required={!central?.vinculado || url !== central.url} type="password" autoComplete="off" value={token} onChange={e => setToken(e.target.value)} /></label>
          <div className="acciones">
            <button className="primario" disabled={ocupado}>{central?.vinculado && !token && url === central.url ? 'Continuar' : 'Vincular y continuar'}</button>
            <button type="button" disabled={ocupado} onClick={() => { setSoloExportar(true); setToken(''); ir(1) }}>Sin conexión, solo exportar</button>
          </div>
        </form>}
        {paso === 1 && <>
          <p>Conecta esta computadora a la misma red que el DVR y busca equipos disponibles.</p>
          {redes.length > 0 && <p className="detalle">Redes disponibles: {redes.join(', ')}</p>}
          <div className="acciones">
            <button className="primario" disabled={ocupado || !!busqueda} onClick={buscar}>{barrido?.terminado ? 'Buscar de nuevo' : 'Buscar DVR'}</button>
            <button disabled={ocupado} onClick={() => seleccionar()}>Agregar a mano</button>
          </div>
          {(busqueda || barrido?.terminado) && <div className="pila progreso">
            <label>Progreso de búsqueda<progress value={barrido?.hechos ?? 0} max={barrido?.total || 1} /></label>
            <p role="status">{barrido?.terminado ? 'Búsqueda terminada.' : 'Buscando equipos…'} {barrido?.total ? `${barrido.hechos} de ${barrido.total} direcciones revisadas.` : ''}</p>
          </div>}
          {barrido?.hallazgos?.map(h => <div key={h.ip} className="hallazgo acciones">
            <div><h3>{h.marca || 'Marca desconocida'} {h.modelo}</h3><p>{h.ip}</p><p className="detalle">{h.pista}</p></div>
            <button className="al-final" onClick={() => seleccionar(h)}>Usar este DVR</button>
          </div>)}
          {barrido?.terminado && !barrido.hallazgos?.length && <p>No se encontraron equipos. Puedes agregar un DVR a mano.</p>}
        </>}
        {paso === 2 && <form className="pila" onSubmit={identificar}>
          <p>Usa el usuario y la contraseña con los que entras al DVR.</p>
          <CamposDvr dvr={dvr} cambiar={setDvr} />
          <button className="primario" disabled={ocupado}>Identificar y ver cámaras</button>
        </form>}
        {paso === 3 && identificacion && <form className="pila" onSubmit={e => { e.preventDefault(); ir(4) }}>
          <p>Revisa la imagen de cada cámara y elige cuáles se verán en la vista general de la tele.</p>
          <Advertencias datos={identificacion} />
          {sinCanales && <p className="mensaje advertencia">El DVR no informó sus canales. Agrega los que estén conectados.</p>}
          <div className="vistas-previas">
            {camaras.map((c, i) => <article className="fila-camara pila" key={i}>
              <h3>Canal {c.canal}</h3>
              <Foto key={c.canal} dvr={dvr} canal={c.canal} />
              <CamposCamara camara={c} editable={sinCanales} cambiar={n => setCamaras(actual => actual.map((a, j) => i === j ? n : a))} />
              {sinCanales && camaras.length > 1 && <button type="button" onClick={() => setCamaras(actual => actual.filter((_, j) => i !== j))}>Quitar canal</button>}
            </article>)}
          </div>
          <div className="acciones">
            {sinCanales && <button type="button" onClick={() => {
              const canal = Math.max(...camaras.map(c => c.canal)) + 1
              setCamaras([...camaras, { canal, nombre: `Cámara ${canal}`, clave: false }])
            }}>Agregar canal</button>}
            <button className="primario" onClick={e => {
              if (new Set(camaras.map(c => c.canal)).size !== camaras.length) {
                e.preventDefault()
                setError('Cada cámara debe usar un canal distinto.')
              }
            }}>Continuar al envío</button>
          </div>
        </form>}
        {paso === 4 && <form className="pila" onSubmit={e => {
          e.preventDefault()
          const boton = (e.nativeEvent as SubmitEvent).submitter
          const exportar = soloExportar || (boton instanceof HTMLButtonElement && boton.value === 'exportar')
          if (!exportar && reemplazo && !confirm(`¿Reemplazar ${reemplazo.nombre} en la central?`)) return
          finalizar(exportar ? 'exportar' : 'enviar')
        }}>
          <CamposSucursal datos={sucursal} cambiar={setSucursal} />
          {aviso && <p className="mensaje advertencia" role="status">No se pudo comprobar si el rango está repetido. {aviso}</p>}
          {repetidas.map(s => <p key={s.id} className="mensaje advertencia">{s.nombre} ya usa direcciones {redDvr}.x. Si ambas sedes tienen el mismo rango, la central podría llegar al DVR equivocado.</p>)}
          {reemplazo && <p className="mensaje advertencia">El identificador ya pertenece a {reemplazo.nombre}. Si envías la configuración, reemplazará esa sucursal.</p>}
          <div className="acciones">
            {!soloExportar && <button className="primario" disabled={ocupado}>Enviar a la central</button>}
            <button className={soloExportar ? 'primario' : undefined} value="exportar" disabled={ocupado}>Exportar archivo</button>
          </div>
        </form>}
      </section>
      {paso > 0 && <button disabled={ocupado} onClick={() => ir(paso - 1)}>Atrás</button>}
    </div>}
  </Marco>
}

function Foto({ dvr, canal }: { dvr: Dvr; canal: number }) {
  const [foto, setFoto] = useState('')
  const [error, setError] = useState('')
  useEffect(() => {
    const control = new AbortController()
    const parametros = new URLSearchParams({
      host: dvr.host, marca: dvr.marca, puerto: String(dvr.puerto), usuario: dvr.usuario, canal: String(canal),
    })
    let actual = ''
    let siguiente: ReturnType<typeof setTimeout>

    async function cargar() {
      const inicio = performance.now()
      try {
        const res = await respuesta(instalador + '/foto?' + parametros, {
          headers: { 'X-DVR-Password': dvr.password },
          signal: AbortSignal.any([control.signal, AbortSignal.timeout(5000)]),
        })
        const blob = await res.blob()
        if (control.signal.aborted) return
        const anterior = actual
        actual = URL.createObjectURL(blob)
        setFoto(actual)
        setError('')
        if (anterior) URL.revokeObjectURL(anterior)
      } catch (e) {
        if (!control.signal.aborted) setError((e as Error).message)
      } finally {
        if (!control.signal.aborted) siguiente = setTimeout(cargar, Math.max(0, 2000 - (performance.now() - inicio)))
      }
    }

    if (canal > 0) void cargar()
    return () => {
      control.abort()
      clearTimeout(siguiente)
      if (actual) URL.revokeObjectURL(actual)
    }
  }, [dvr, canal])

  return <div className="foto">
    {foto ? <img src={foto} alt={`Vista previa del canal ${canal}`} /> : <div className="foto-vacia">{error ? 'Imagen no disponible' : 'Cargando imagen…'}</div>}
    {error && <small className="problema">{error}</small>}
  </div>
}
