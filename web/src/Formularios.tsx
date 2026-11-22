import type { Camara, Dvr, Identificacion, Sucursal } from './api'

export function Marco({ titulo, children }: { titulo: string; children: React.ReactNode }) {
  return <div className="gestion">
    <header className="gestion-cabecera"><a href="/">Monitor de cámaras</a></header>
    <main className="gestion-cuerpo"><h1>{titulo}</h1>{children}</main>
  </div>
}

export function Mensajes({ error, aviso }: { error?: string; aviso?: string }) {
  return <>
    {error && <p className="mensaje problema" role="alert">{error}</p>}
    {aviso && <p className="mensaje" role="status">{aviso}</p>}
  </>
}

export function CamposSucursal({ datos, cambiar, fijo = false }: {
  datos: Pick<Sucursal, 'id' | 'nombre'>; cambiar: (datos: Pick<Sucursal, 'id' | 'nombre'>) => void; fijo?: boolean
}) {
  return <div className="campos">
    <label>Nombre de la sucursal<input required value={datos.nombre} onChange={e => cambiar({ ...datos, nombre: e.target.value })} /></label>
    <label>Identificador<input required readOnly={fijo} pattern="[a-z0-9_]+" value={datos.id} onChange={e => cambiar({ ...datos, id: e.target.value })} />
      <small>Usa minúsculas, números o guion bajo. Por ejemplo, sucursal_centro.</small>
    </label>
  </div>
}

export function CamposDvr({ dvr, cambiar, conservar = false }: {
  dvr: Dvr; cambiar: (dvr: Dvr) => void; conservar?: boolean
}) {
  return <div className="campos">
    <label>Dirección IP del DVR<input required value={dvr.host} onChange={e => cambiar({ ...dvr, host: e.target.value.trim() })} /></label>
    <label>Marca<select value={dvr.marca} onChange={e => cambiar({ ...dvr, marca: e.target.value })}>
      {Object.entries({ hikvision: 'Hikvision', dahua: 'Dahua', uniview: 'Uniview', xmeye: 'XMEye', onvif: 'ONVIF', rtsp: 'RTSP genérico' })
        .map(([id, nombre]) => <option key={id} value={id}>{nombre}</option>)}
    </select></label>
    <label>Puerto (0 para usar el habitual)<input type="number" min="0" max="65535" required value={dvr.puerto} onChange={e => cambiar({ ...dvr, puerto: e.target.valueAsNumber || 0 })} /></label>
    <label>Usuario del DVR<input required autoComplete="off" value={dvr.usuario} onChange={e => cambiar({ ...dvr, usuario: e.target.value })} /></label>
    <label>Contraseña del DVR<input type="password" autoComplete="new-password" value={dvr.password} onChange={e => cambiar({ ...dvr, password: e.target.value })} />
      {conservar && dvr.tienePassword && <small>Déjala vacía para conservar la contraseña guardada.</small>}
    </label>
    {dvr.marca === 'rtsp' && <>
      <label>Ruta de video para la cuadrícula<input required value={dvr.rutaSub} onChange={e => cambiar({ ...dvr, rutaSub: e.target.value })} /><small>Usa {'{canal}'} donde va el número de cámara.</small></label>
      <label>Ruta de video para pantalla grande (opcional)<input value={dvr.rutaPrincipal} onChange={e => cambiar({ ...dvr, rutaPrincipal: e.target.value })} /></label>
    </>}
  </div>
}

export function CamposCamara({ camara, cambiar, editable = false }: {
  camara: Camara; cambiar: (c: Camara) => void; editable?: boolean
}) {
  return <div className="campos">
    {editable && <label>Canal<input required type="number" min="1" value={camara.canal} onChange={e => cambiar({ ...camara, canal: e.target.valueAsNumber || 0 })} /></label>}
    <label>Nombre de la cámara<input required value={camara.nombre} onChange={e => cambiar({ ...camara, nombre: e.target.value })} /></label>
    <label className="casilla"><input type="checkbox" checked={camara.clave} onChange={e => cambiar({ ...camara, clave: e.target.checked })} /> Mostrar en la vista general</label>
  </div>
}

const explicaciones: Record<string, string> = {
  h265: 'H.265 es un formato de video que algunos navegadores no pueden mostrar.',
  dhcp: 'DHCP asigna la IP automáticamente. Si el módem cambia esa dirección, la central puede perder conexión con el DVR.',
  sin_rtsp: 'RTSP es la forma en que el monitor recibe el video del DVR.',
  resolucion_alta: 'La vista general usa una imagen más ligera para mostrar varias cámaras a la vez.',
}

export function Advertencias({ datos }: { datos: Identificacion }) {
  return datos.advertencias?.map(({ codigo, mensaje, nivel }, i) =>
    <div key={i} className={`mensaje ${nivel === 'error' ? 'problema' : 'advertencia'}`}>
      <strong>{mensaje}</strong>
      {explicaciones[codigo] && <div>{explicaciones[codigo]}</div>}
    </div>,
  )
}
