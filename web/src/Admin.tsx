import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { admin, api, dvrNuevo, ErrorApi, solicitud, type Ajustes, type Identificacion, type Sucursal, type Vinculo } from './api'
import { Advertencias, CamposCamara, CamposDvr, CamposSucursal, Marco, Mensajes } from './Formularios'

type Inicio = { configurado: boolean; sesion: boolean }
type Pestaña = 'Sucursales' | 'Ajustes' | 'Vínculos'

export default function Admin() {
  const [inicio, setInicio] = useState<Inicio>()
  const [error, setError] = useState('')
  const [ocupado, setOcupado] = useState(false)
  const [password, setPassword] = useState('')

  const consultar = useCallback(() => {
    void api<Inicio>(admin + '/inicio').then(setInicio).catch((e: Error) => setError(e.message))
  }, [])
  useEffect(consultar, [consultar])
  const salir = useCallback((mensaje = '') => {
    setInicio({ configurado: true, sesion: false })
    setError(mensaje)
  }, [])

  async function entrar(e: FormEvent) {
    e.preventDefault()
    setOcupado(true)
    setError('')
    try {
      await api(admin + (inicio?.configurado ? '/login' : '/configurar'), solicitud('POST', { password }))
      setInicio({ configurado: true, sesion: true })
      setPassword('')
    } catch (e) {
      if (e instanceof ErrorApi && e.estado === 409) setInicio({ configurado: true, sesion: false })
      setError((e as Error).message)
    } finally {
      setOcupado(false)
    }
  }

  return <Marco titulo="Panel de administración">
    {inicio?.sesion ? <Panel salir={salir} /> : <>
      <Mensajes error={error} />
      {!inicio && (error ? <button onClick={() => { setError(''); consultar() }}>Reintentar</button> : <p>Cargando…</p>)}
      {inicio && <form className="tarjeta acceso pila" onSubmit={entrar}>
        <h2>{inicio.configurado ? 'Iniciar sesión' : 'Crear contraseña'}</h2>
        <label>Contraseña<input required type="password" minLength={inicio.configurado ? undefined : 8}
          autoComplete={inicio.configurado ? 'current-password' : 'new-password'} value={password}
          onChange={e => setPassword(e.target.value)} /></label>
        {!inicio.configurado && <small>Usa al menos 8 caracteres. Esta contraseña protege el panel.</small>}
        <button className="primario" disabled={ocupado}>{inicio.configurado ? 'Entrar' : 'Crear contraseña'}</button>
      </form>}
    </>}
  </Marco>
}

function Panel({ salir }: { salir: (mensaje?: string) => void }) {
  const [pestaña, setPestaña] = useState<Pestaña>('Sucursales')
  const [sucursales, setSucursales] = useState<Sucursal[]>()
  const [ajustes, setAjustes] = useState<Ajustes>()
  const [vinculos, setVinculos] = useState<Vinculo[]>()
  const [edicion, setEdicion] = useState<{ original?: string; datos: Sucursal }>()
  const [identificacion, setIdentificacion] = useState<{ id: string; datos: Identificacion }>()
  const [nombre, setNombre] = useState('')
  const [token, setToken] = useState('')
  const campoToken = useRef<HTMLInputElement>(null)
  const [error, setError] = useState('')
  const [aviso, setAviso] = useState('')
  const [ocupado, setOcupado] = useState(false)

  const ejecutar = useCallback(async (accion: () => Promise<void>) => {
    setOcupado(true)
    setError('')
    setAviso('')
    try {
      await accion()
    } catch (e) {
      if (e instanceof ErrorApi && e.estado === 401) salir(e.message)
      else setError((e as Error).message)
    } finally {
      setOcupado(false)
    }
  }, [salir])

  const cargarSucursales = useCallback(async () => setSucursales(await api<Sucursal[]>(admin + '/sucursales') ?? []), [])
  const cargarVinculos = useCallback(async () => setVinculos(await api<Vinculo[]>(admin + '/vinculos') ?? []), [])

  const cargar = useCallback(() => {
    void ejecutar(async () => {
      if (pestaña === 'Sucursales') await cargarSucursales()
      if (pestaña === 'Ajustes') setAjustes(await api<Ajustes>(admin + '/ajustes'))
      if (pestaña === 'Vínculos') await cargarVinculos()
    })
  }, [pestaña, ejecutar, cargarSucursales, cargarVinculos])
  useEffect(() => {
    const inicio = setTimeout(cargar, 0)
    return () => clearTimeout(inicio)
  }, [cargar])

  function operar(s: Sucursal, accion: 'borrar' | 'reiniciar' | 'identificar') {
    if (accion === 'borrar' && !confirm(`¿Borrar ${s.nombre} del monitor?`)) return
    if (accion === 'reiniciar' && !confirm(`¿Reiniciar el DVR de ${s.nombre}? Las cámaras se desconectarán unos minutos.`)) return
    void ejecutar(async () => {
      const ruta = admin + '/sucursales/' + encodeURIComponent(s.id)
      if (accion === 'identificar') {
        const datos = await api<Identificacion>(ruta + '/identificar', solicitud('POST'))
        setIdentificacion({ id: s.id, datos })
      } else {
        await api(accion === 'borrar' ? ruta : ruta + '/reiniciar', solicitud(accion === 'borrar' ? 'DELETE' : 'POST'))
        setAviso(accion === 'borrar' ? 'Sucursal borrada.' : 'Se solicitó el reinicio del DVR.')
      }
      await cargarSucursales()
    })
  }

  function guardarSucursal(e: FormEvent) {
    e.preventDefault()
    if (!edicion) return
    const { original, datos } = edicion
    void ejecutar(async () => {
      if (!datos.camaras.length) throw new Error('Agrega al menos una cámara.')
      if (new Set(datos.camaras.map(c => c.canal)).size !== datos.camaras.length) throw new Error('Cada cámara debe usar un canal distinto.')
      await api(admin + '/sucursales' + (original ? '/' + encodeURIComponent(original) : ''), solicitud(original ? 'PUT' : 'POST', datos))
      setEdicion(undefined)
      setAviso('Sucursal guardada.')
      await cargarSucursales()
    })
  }

  return <div className="pila">
    <nav className="acciones navegacion" aria-label="Secciones del panel">
      {(['Sucursales', 'Ajustes', 'Vínculos'] as const).map(p => <button key={p} disabled={ocupado}
        aria-current={pestaña === p ? 'page' : undefined} onClick={() => {
          setPestaña(p)
          setEdicion(undefined)
          setIdentificacion(undefined)
          if (p !== pestaña) { setToken(''); setNombre('') }
        }}>{p}</button>)}
      <button className="al-final" disabled={ocupado} onClick={() => void ejecutar(async () => {
        await api(admin + '/logout', solicitud('POST'))
        salir()
      })}>Cerrar sesión</button>
    </nav>
    <Mensajes error={error} aviso={aviso} />
    {error && <button onClick={cargar} disabled={ocupado}>Actualizar sección</button>}
    {pestaña === 'Sucursales' && (edicion ? <form className="tarjeta pila" onSubmit={guardarSucursal}>
      <h2>{edicion.original ? 'Editar sucursal' : 'Agregar sucursal'}</h2>
      <fieldset className="pila" disabled={ocupado}>
        <CamposSucursal datos={edicion.datos} fijo={!!edicion.original} cambiar={datos => setEdicion({ ...edicion, datos: { ...edicion.datos, ...datos } })} />
        <CamposDvr dvr={edicion.datos.dvr} conservar={!!edicion.original} cambiar={dvr => setEdicion({ ...edicion, datos: { ...edicion.datos, dvr } })} />
        <h3>Cámaras</h3>
        {edicion.datos.camaras.map((camara, i) => <div className="fila-camara pila" key={i}>
          <CamposCamara camara={camara} editable cambiar={c => setEdicion({
            ...edicion, datos: { ...edicion.datos, camaras: edicion.datos.camaras.map((actual, j) => j === i ? c : actual) },
          })} />
          <button type="button" onClick={() => setEdicion({ ...edicion, datos: { ...edicion.datos, camaras: edicion.datos.camaras.filter((_, j) => j !== i) } })}>Quitar cámara</button>
        </div>)}
        <div className="acciones">
          <button type="button" onClick={() => {
            const camaras = edicion.datos.camaras
            const canal = Math.max(0, ...camaras.map(c => c.canal)) + 1
            setEdicion({ ...edicion, datos: { ...edicion.datos, camaras: [...camaras, { canal, nombre: `Cámara ${canal}`, clave: !camaras.length }] } })
          }}>Agregar cámara</button>
          <button type="button" onClick={() => setEdicion(undefined)}>Cancelar</button>
          <button className="primario">Guardar sucursal</button>
        </div>
      </fieldset>
    </form> : <>
      <div className="acciones">
        <h2>Sucursales</h2>
        <button className="primario al-final" disabled={ocupado} onClick={() => setEdicion({
          datos: { id: '', nombre: '', dvr: dvrNuevo, camaras: [{ canal: 1, nombre: 'Cámara 1', clave: true }] },
        })}>Agregar sucursal</button>
      </div>
      {sucursales?.length === 0 && <p className="tarjeta">Sin sucursales todavía.</p>}
      {sucursales?.map(s => <article className="tarjeta pila" key={s.id}>
        <div className="acciones"><h3>{s.nombre}</h3>
          <span className={`estado al-final ${s.enLinea ? 'conectado' : ''}`}>{s.enLinea === undefined ? 'Estado pendiente' : s.enLinea ? 'En línea' : 'Sin conexión'}</span>
        </div>
        <p className="detalle">{s.dvr.marca} · {s.dvr.host} · {s.camaras.length} {s.camaras.length === 1 ? 'cámara' : 'cámaras'}</p>
        {s.error && <p className="detalle">{s.error}</p>}
        <div><strong>Huella del DVR</strong>
          <p className="detalle">{[s.huella?.modelo, s.huella?.serie && `Serie ${s.huella.serie}`, s.huella?.mac && `MAC ${s.huella.mac}`].filter(Boolean).join(' · ') || 'Pendiente de identificar'}</p>
          {s.huella?.actualizada && <p className="detalle">Actualizada el {new Date(s.huella.actualizada).toLocaleString('es-MX')}</p>}
        </div>
        {identificacion?.id === s.id && <Advertencias datos={identificacion.datos} />}
        <div className="acciones">
          <button disabled={ocupado} onClick={() => setEdicion({ original: s.id, datos: { ...s, dvr: { ...s.dvr, password: '' } } })}>Editar</button>
          <button disabled={ocupado} onClick={() => operar(s, 'identificar')}>Identificar DVR</button>
          <button disabled={ocupado} onClick={() => operar(s, 'reiniciar')}>Reiniciar DVR</button>
          <button className="peligro" disabled={ocupado} onClick={() => operar(s, 'borrar')}>Borrar</button>
        </div>
      </article>)}
    </>)}
    {pestaña === 'Ajustes' && ajustes && <form className="tarjeta pila" onSubmit={e => {
      e.preventDefault()
      void ejecutar(async () => {
        await api(admin + '/ajustes', solicitud('PUT', ajustes))
        setAjustes(await api<Ajustes>(admin + '/ajustes'))
        setAviso('Ajustes guardados.')
      })
    }}>
      <fieldset disabled={ocupado} className="pila">
        <h2>Rotación de la tele</h2>
        <div className="campos">
          <label>Segundos por vista<input type="number" required min="1" value={ajustes.interfaz.rotacionSegundos} onChange={e => setAjustes({ ...ajustes, interfaz: { ...ajustes.interfaz, rotacionSegundos: e.target.valueAsNumber || 0 } })} /></label>
          <label>Segundos antes de volver a la cuadrícula<input type="number" required min="1" value={ajustes.interfaz.volverACuadriculaSegundos} onChange={e => setAjustes({ ...ajustes, interfaz: { ...ajustes.interfaz, volverACuadriculaSegundos: e.target.valueAsNumber || 0 } })} /></label>
          <label className="casilla"><input type="checkbox" checked={ajustes.interfaz.rotarVistaGeneral} onChange={e => setAjustes({ ...ajustes, interfaz: { ...ajustes.interfaz, rotarVistaGeneral: e.target.checked } })} /> Incluir vista general en la rotación</label>
        </div>
        <h2>Alertas por Telegram</h2>
        <div className="campos">
          <label>ID del chat<input value={ajustes.telegram.chatId} onChange={e => setAjustes({ ...ajustes, telegram: { ...ajustes.telegram, chatId: e.target.value } })} /></label>
          <label>Token del bot<input type="password" autoComplete="new-password" value={ajustes.telegram.token ?? ''} onChange={e => setAjustes({ ...ajustes, telegram: { ...ajustes.telegram, token: e.target.value } })} />
            {ajustes.telegram.tieneToken && <small>Déjalo vacío para conservar el token guardado.</small>}
          </label>
        </div>
        <div className="acciones">
          <button className="primario">Guardar ajustes</button>
          <button type="button" onClick={() => void ejecutar(async () => {
            await api(admin + '/telegram/prueba', solicitud('POST'))
            setAviso('Mensaje de prueba enviado.')
          })}>Probar Telegram</button>
        </div>
        <small>La prueba usa los ajustes guardados.</small>
      </fieldset>
    </form>}
    {pestaña === 'Vínculos' && <>
      <form className="tarjeta pila" onSubmit={e => {
        e.preventDefault()
        void ejecutar(async () => {
          const nuevo = await api<Vinculo & { token: string }>(admin + '/vinculos', solicitud('POST', { nombre }))
          setToken(nuevo.token)
          setNombre('')
          await cargarVinculos()
        })
      }}>
        <h2>Crear vínculo</h2>
        <label>Nombre del vínculo<input required value={nombre} onChange={e => setNombre(e.target.value)} /></label>
        <button className="primario" disabled={ocupado}>Crear vínculo</button>
      </form>
      {token && <div className="tarjeta pila">
        <label>Token para pegar en el asistente<input ref={campoToken} readOnly value={token} onFocus={e => e.target.select()} /></label>
        <small>Cópialo ahora. Solo se muestra esta vez.</small>
        <div className="acciones"><button onClick={() => void ejecutar(async () => {
          if (navigator.clipboard) await navigator.clipboard.writeText(token)
          else {
            campoToken.current?.select()
            if (!document.execCommand('copy')) throw new Error('Selecciona el token y cópialo con el teclado.')
          }
          setAviso('Token copiado.')
        })}>Copiar token</button><button onClick={() => setToken('')}>Ocultar token</button></div>
      </div>}
      {vinculos?.length === 0 && <p className="tarjeta">Todavía no hay vínculos.</p>}
      {vinculos?.map(v => <article className="tarjeta acciones" key={v.id}>
        <div><h3>{v.nombre}</h3><p className="detalle">{v.ultimoUso && !v.ultimoUso.startsWith('0001') ? `Último uso: ${new Date(v.ultimoUso).toLocaleString('es-MX')}` : 'Sin uso todavía'}</p></div>
        <button className="al-final peligro" disabled={ocupado} onClick={() => {
          if (confirm(`¿Revocar el vínculo ${v.nombre}?`)) void ejecutar(async () => {
            await api(admin + '/vinculos/' + encodeURIComponent(v.id), solicitud('DELETE'))
            setToken('')
            await cargarVinculos()
          })
        }}>Revocar</button>
      </article>)}
    </>}
  </div>
}
