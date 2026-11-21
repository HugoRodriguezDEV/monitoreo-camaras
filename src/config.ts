// Configuración del monitor. Vive en public/config.json para que se puedan
// agregar sucursales, cámaras o cambiar el orden sin recompilar.

export type Camara = {
  /** Nombre del stream en go2rtc (substream, para la cuadrícula). */
  stream: string
  /** Stream principal en go2rtc, para la vista en grande. Si falta, se usa `stream`. */
  streamHd?: string
  nombre: string
  /** Aparece en la vista general. */
  clave?: boolean
}

export type Sucursal = {
  id: string
  nombre: string
  camaras: Camara[]
}

export type Config = {
  /** Base de go2rtc. Vacío = mismo origen (go2rtc sirve la interfaz, o el proxy de Vite en desarrollo). */
  go2rtc: string
  rotacionSegundos: number
  /** La vista general entra en la rotación junto con las sucursales. */
  rotarVistaGeneral: boolean
  /** Tras cuántos segundos la cámara en grande vuelve a la cuadrícula. */
  volverACuadriculaSegundos: number
  /** Segundos sin imagen en todas las cámaras de una sucursal para marcarla sin conexión. */
  sinConexionSegundos: number
  sucursales: Sucursal[]
}

const DEFAULTS: Omit<Config, 'sucursales'> = {
  go2rtc: '',
  rotacionSegundos: 25,
  rotarVistaGeneral: true,
  volverACuadriculaSegundos: 60,
  sinConexionSegundos: 15,
}

export async function cargarConfig(): Promise<Config> {
  const res = await fetch(`${import.meta.env.BASE_URL}config.json`, { cache: 'no-store' })
  if (!res.ok) throw new Error(`No se pudo leer config.json (${res.status})`)
  const json = (await res.json()) as Partial<Config>
  if (!Array.isArray(json.sucursales) || json.sucursales.length === 0) {
    throw new Error('config.json no tiene sucursales')
  }
  return { ...DEFAULTS, ...json, sucursales: json.sucursales }
}

/** URL del WebSocket de go2rtc para un stream. */
export function urlStream(config: Config, stream: string): string {
  const base = config.go2rtc || location.origin
  return `${base.replace(/^http/, 'ws')}/api/ws?src=${encodeURIComponent(stream)}`
}

/** Carga el reproductor de go2rtc (video-stream.js) una sola vez. */
let reproductor: Promise<unknown> | undefined
export function cargarReproductor(config: Config): Promise<unknown> {
  reproductor ??= new Promise((resolve, reject) => {
    const script = document.createElement('script')
    script.type = 'module'
    script.src = `${config.go2rtc}/video-stream.js`
    script.onerror = () => reject(new Error(`No se pudo cargar el reproductor desde go2rtc (${script.src})`))
    document.head.appendChild(script)
    customElements.whenDefined('video-stream').then(resolve)
  })
  return reproductor
}
