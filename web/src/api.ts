export type Camara = { canal: number; nombre: string; clave: boolean }
export type Dvr = {
  host: string; marca: string; puerto: number; usuario: string; password: string
  tienePassword?: boolean; rutaSub: string; rutaPrincipal: string
}
export type Huella = { marca?: string; modelo?: string; serie?: string; mac?: string; actualizada?: string }
export type Sucursal = {
  id: string; nombre: string; dvr: Dvr; camaras: Camara[]
  huella?: Huella; enLinea?: boolean; error?: string
}
export type Identificacion = {
  identidad: Huella & { canales?: (Camara & { conectado: boolean })[] }
  advertencias?: { nivel: 'aviso' | 'error'; codigo: string; mensaje: string }[]
}
export type Ajustes = {
  interfaz: { rotacionSegundos: number; rotarVistaGeneral: boolean; volverACuadriculaSegundos: number }
  telegram: { chatId: string; tieneToken?: boolean; token?: string }
}
export type Vinculo = { id: string; nombre: string; creado?: string; ultimoUso?: string }
export const admin = '/api/admin'
export const instalador = '/api/instalador'
export const dvrNuevo: Dvr = {
  host: '', marca: 'hikvision', puerto: 0, usuario: 'admin', password: '', rutaSub: '', rutaPrincipal: '',
}

export class ErrorApi extends Error {
  constructor(mensaje: string, estado: number) {
    super(mensaje)
    this.estado = estado
  }
  readonly estado: number
}

export async function respuesta(ruta: string, opciones?: RequestInit) {
  let res: Response
  try {
    res = await fetch(ruta, { cache: 'no-store', ...opciones })
  } catch {
    throw new Error('No se pudo conectar con el servidor. Revisa la conexión e inténtalo de nuevo.')
  }
  if (!res.ok) {
    const datos = await res.json().catch(() => null)
    throw new ErrorApi(datos?.error || `El servidor respondió con un error (${res.status}).`, res.status)
  }
  return res
}

export function solicitud(method: string, cuerpo?: unknown): RequestInit {
  return cuerpo === undefined ? { method } : {
    method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(cuerpo),
  }
}

export async function api<T = void>(ruta: string, opciones?: RequestInit): Promise<T> {
  const res = await respuesta(ruta, opciones)
  const cuerpo = await res.text()
  return cuerpo ? JSON.parse(cuerpo) as T : undefined as T
}
