import { createServer } from 'node:http'
import { randomUUID } from 'node:crypto'

const puerto = Number(process.env.API_FALSA_PUERTO ?? 8090)
const ahora = () => new Date().toISOString()
const dvr = { marca: 'dahua', host: '192.168.1.108', puerto: 0, usuario: 'admin', password: 'demo', rutaSub: '', rutaPrincipal: '' }
const canales = [
  { canal: 1, nombre: 'Entrada', clave: true },
  { canal: 2, nombre: 'Caja', clave: false },
]
const huella = { marca: 'dahua', modelo: 'XVR5104', serie: 'DEMO-5104', mac: '02:00:00:00:00:01', actualizada: ahora() }
const sucursales = new Map([
  ['centro', { id: 'centro', nombre: 'Sucursal Centro', dvr, camaras: canales, huella, enLinea: true, error: '' }],
  ['norte', { id: 'norte', nombre: 'Sucursal Norte', dvr: { ...dvr, host: '192.168.2.64', marca: 'hikvision' }, camaras: canales.slice(0, 1), enLinea: false, error: 'El DVR no responde.' }],
])
let interfaz = { rotacionSegundos: 25, rotarVistaGeneral: true, volverACuadriculaSegundos: 60 }
let telegram = { chatId: '', token: '' }
let password = ''
let version = 1
let central = { url: '', nombre: '', token: '' }
const sesiones = new Set()
const intentos = new Map()
const vinculos = new Map()
const barridos = new Map()
const hallazgos = [
  { ip: dvr.host, mac: huella.mac, marca: 'dahua', modelo: huella.modelo, puertos: [80, 554, 37777], onvif: true, pista: 'El equipo se anunció como Dahua.' },
  { ip: '192.168.1.64', mac: '', marca: 'hikvision', modelo: 'DS-7204', puertos: [80, 554, 8000], onvif: false, pista: 'La página de acceso se identificó como Hikvision.' },
]

function identificar(marca) {
  return {
    identidad: { ...huella, marca, firmware: 'Demostración', canales: canales.map(c => ({
      ...c, conectado: true, codecSub: c.canal === 2 ? 'H.265' : 'H.264', resolucionSub: '640x360', fpsSub: 15,
    })), dhcp: true },
    advertencias: [
      { nivel: 'aviso', codigo: 'h265', mensaje: 'La cámara 2 manda H.265. En los ajustes de video del DVR, cambia el formato de esa cámara a H.264.' },
      { nivel: 'aviso', codigo: 'dhcp', mensaje: 'El DVR recibe su IP del módem. Reserva la dirección del DVR en el módem para que siga siendo la misma.' },
    ],
  }
}

function publica(s) {
  return { ...s, dvr: { ...s.dvr, password: '', tienePassword: Boolean(s.dvr.password) } }
}

function reducidas() {
  return [...sucursales.values()].map(s => ({ id: s.id, nombre: s.nombre, dvr: { host: s.dvr.host } }))
}

async function recibir(req) {
  let cuerpo = ''
  for await (const parte of req) cuerpo += parte
  return cuerpo ? JSON.parse(cuerpo) : {}
}

function validacion(s) {
  if (!/^[a-z0-9_]+$/.test(s.id ?? '') || !s.nombre?.trim()) return 'Escribe un identificador válido y el nombre de la sucursal.'
  if (!s.dvr?.host || !s.dvr.marca || (s.dvr.marca === 'rtsp' && !s.dvr.rutaSub)) return 'Completa los datos del DVR.'
  if (!Array.isArray(s.camaras) || !s.camaras.length || s.camaras.some(c => !Number.isInteger(c.canal) || c.canal < 1 || !c.nombre?.trim()) ||
    new Set(s.camaras.map(c => c.canal)).size !== s.camaras.length) return 'Agrega cámaras con nombre y números de canal distintos.'
  return ''
}

function guardar(s, conservar) {
  const anterior = sucursales.get(s.id)
  sucursales.set(s.id, {
    ...s,
    dvr: { ...s.dvr, password: s.dvr.password || (conservar ? anterior?.dvr.password ?? '' : '') },
    enLinea: anterior?.enLinea ?? true, error: anterior?.error ?? '',
  })
  version++
  return publica(sucursales.get(s.id))
}

async function atender(req, res) {
  const { pathname, searchParams } = new URL(req.url, 'http://localhost')
  const ruta = pathname.split('/').slice(1).map(decodeURIComponent)
  const metodo = req.method
  const json = (datos, estado = 200) => {
    res.writeHead(estado, { 'Content-Type': 'application/json; charset=utf-8', 'Cache-Control': 'no-store' })
    res.end(JSON.stringify(datos))
  }
  const error = (estado, mensaje) => json({ error: mensaje }, estado)
  const cookie = req.headers.cookie?.match(/(?:^|;\s*)monitor_sesion=([^;]+)/)?.[1]
  const autorizado = sesiones.has(cookie)
  const abrir = () => {
    const id = randomUUID()
    sesiones.add(id)
    res.setHeader('Set-Cookie', `monitor_sesion=${id}; Path=/; HttpOnly; SameSite=Strict; Max-Age=43200`)
    return json({ ok: true })
  }

  if (pathname === '/config.json' && metodo === 'GET') return json({
    go2rtc: '', ...interfaz, sinConexionSegundos: 30,
    sucursales: [...sucursales.values()].map(s => ({ id: s.id, nombre: s.nombre, camaras: s.camaras.map(c => ({
      nombre: c.nombre, clave: c.clave, stream: `${s.id}_c${c.canal}`, streamHd: `${s.id}_c${c.canal}_hd`,
    })) })),
  })
  if (pathname === '/api/estado' && metodo === 'GET') return json({
    actualizado: ahora(), configVersion: version, go2rtc: { enLinea: true },
    sucursales: [...sucursales.values()].map(({ id, enLinea, error: detalle }) => ({ id, enLinea, desde: ahora(), latenciaMs: 3, error: detalle })),
  })
  if (pathname === '/video-stream.js' && metodo === 'GET') {
    res.writeHead(200, { 'Content-Type': 'text/javascript' })
    return res.end("customElements.define('video-stream', class extends HTMLElement { connectedCallback() { this.textContent = 'Video de demostración' } });")
  }

  const [, superficie, recurso, id, accion] = ruta
  if (superficie === 'admin') {
    if (recurso === 'inicio' && metodo === 'GET') return json({ configurado: Boolean(password), sesion: autorizado })
    if (recurso === 'configurar' && metodo === 'POST') {
      const datos = await recibir(req)
      if (password) return error(409, 'La contraseña ya está configurada.')
      if (typeof datos.password !== 'string' || datos.password.length < 8) return error(400, 'Usa una contraseña de al menos 8 caracteres.')
      password = datos.password
      return abrir()
    }
    if (recurso === 'login' && metodo === 'POST') {
      const ip = req.socket.remoteAddress
      const registro = intentos.get(ip)
      if (registro?.fallos >= 5 && Date.now() < registro.hasta) return error(429, 'Demasiados intentos. Espera cinco minutos.')
      const datos = await recibir(req)
      if (!password || datos.password !== password) {
        intentos.set(ip, { fallos: (registro && Date.now() < registro.hasta ? registro.fallos : 0) + 1, hasta: Date.now() + 300_000 })
        return error(401, 'Contraseña incorrecta.')
      }
      intentos.delete(ip)
      return abrir()
    }
    if (!autorizado) return error(401, 'Inicia sesión para continuar.')
    if (recurso === 'logout' && metodo === 'POST') {
      sesiones.delete(cookie)
      res.setHeader('Set-Cookie', 'monitor_sesion=; Path=/; HttpOnly; SameSite=Strict; Max-Age=0')
      return json({ ok: true })
    }
    if (recurso === 'sucursales') {
      if (!id && metodo === 'GET') return json([...sucursales.values()].map(publica))
      if ((!id && metodo === 'POST') || (id && !accion && metodo === 'PUT')) {
        const datos = await recibir(req)
        if (id && !sucursales.has(id)) return error(404, 'La sucursal no existe.')
        if (id && id !== datos.id) return error(400, 'El identificador de la sucursal no coincide.')
        if (!id && sucursales.has(datos.id)) return error(409, 'Ya existe una sucursal con ese identificador.')
        const problema = validacion(datos)
        if (problema) return error(400, problema)
        return json(guardar(datos, true), id ? 200 : 201)
      }
      if (id && !sucursales.has(id)) return error(404, 'La sucursal no existe.')
      if (id && !accion && metodo === 'DELETE') {
        sucursales.delete(id)
        version++
        return json({ ok: true })
      }
      if (id && metodo === 'POST') {
        const sucursal = sucursales.get(id)
        if (accion === 'identificar') {
          const datos = identificar(sucursal.dvr.marca)
          sucursal.huella = { ...huella, marca: sucursal.dvr.marca, actualizada: ahora() }
          version++
          return json(datos)
        }
        if (accion === 'reiniciar') {
          if (sucursal.dvr.marca === 'rtsp') return error(501, 'La marca no permite reiniciar el DVR desde el panel.')
          return json({ ok: true })
        }
      }
    }
    if (recurso === 'ajustes') {
      if (metodo === 'PUT') {
        const datos = await recibir(req)
        if (!datos.interfaz || !datos.telegram) return error(400, 'Completa los ajustes.')
        interfaz = datos.interfaz
        telegram = { chatId: datos.telegram.chatId, token: datos.telegram.token || telegram.token }
        version++
      }
      if (metodo === 'GET' || metodo === 'PUT') return json({ interfaz, telegram: { chatId: telegram.chatId, tieneToken: Boolean(telegram.token) } })
    }
    if (recurso === 'telegram' && id === 'prueba' && metodo === 'POST') {
      return telegram.chatId && telegram.token ? json({ ok: true }) : error(400, 'Guarda el ID del chat y el token del bot primero.')
    }
    if (recurso === 'vinculos') {
      if (metodo === 'GET' && !id) return json([...vinculos.values()].map(v => ({
        id: v.id, nombre: v.nombre, creado: v.creado, ultimoUso: v.ultimoUso,
      })))
      if (metodo === 'POST' && !id) {
        const datos = await recibir(req)
        if (!datos.nombre?.trim()) return error(400, 'Escribe un nombre para el vínculo.')
        const vinculo = { id: randomUUID(), nombre: datos.nombre, token: randomUUID(), creado: ahora() }
        vinculos.set(vinculo.id, vinculo)
        return json({ id: vinculo.id, nombre: vinculo.nombre, token: vinculo.token }, 201)
      }
      if (metodo === 'DELETE' && id) return vinculos.delete(id) ? json({ ok: true }) : error(404, 'El vínculo no existe.')
    }
  }

  if (superficie === 'vinculo') {
    const vinculo = [...vinculos.values()].find(v => req.headers.authorization === 'Bearer ' + v.token)
    if (!vinculo) return error(401, 'El token de vínculo es incorrecto o fue revocado.')
    vinculo.ultimoUso = ahora()
    if (recurso === 'comprobar' && metodo === 'GET') return json({ ok: true, central: 'Central de demostración' })
    if (recurso === 'sucursales' && !id && metodo === 'GET') return json(reducidas())
    if (recurso === 'sucursales' && id && metodo === 'PUT') {
      const datos = await recibir(req)
      if (datos.id !== id) return error(400, 'El identificador de la sucursal no coincide.')
      const problema = validacion(datos)
      return problema ? error(400, problema) : json({ ok: true, sucursal: guardar(datos, false) })
    }
  }

  if (superficie === 'instalador') {
    if (recurso === 'redes' && metodo === 'GET') return json({ redes: ['192.168.1.0/24'] })
    if (recurso === 'buscar' && metodo === 'POST' && !id) {
      await recibir(req)
      const nuevo = randomUUID()
      barridos.set(nuevo, Date.now())
      return json({ id: nuevo })
    }
    if (recurso === 'buscar' && metodo === 'GET' && id) {
      if (!barridos.has(id)) return error(404, 'No se encontró esa búsqueda.')
      const total = 254
      const hechos = Math.min(total, Math.floor((Date.now() - barridos.get(id)) * total / 5000))
      return json({ terminado: hechos === total, hechos, total, hallazgos: hechos === total ? hallazgos : [] })
    }
    if (recurso === 'identificar' && metodo === 'POST') {
      const datos = await recibir(req)
      if (!datos.host || !datos.marca) return error(400, 'Indica la dirección y la marca del DVR.')
      if (datos.password !== 'demo') return error(401, 'Usuario o contraseña incorrectos. Para el DVR de prueba usa la contraseña demo.')
      return json(identificar(datos.marca))
    }
    if (recurso === 'foto' && metodo === 'GET') {
      if (req.headers['x-dvr-password'] !== 'demo') return error(401, 'Contraseña incorrecta en el DVR de prueba.')
      const canal = Number(searchParams.get('canal'))
      if (!Number.isInteger(canal) || canal < 1) return error(400, 'Indica un número de canal válido.')
      const color = canal % 2 ? '#2b5173' : '#365d53'
      res.writeHead(200, { 'Content-Type': 'image/svg+xml', 'Cache-Control': 'no-store' })
      return res.end(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360">
        <rect width="640" height="360" fill="${color}"/>
        <path d="M0 282 196 143h248l196 139v78H0Z" fill="#253745"/>
        <path d="M0 282 196 143V53L0 0Zm640 0L444 143V53L640 0Z" fill="#496c81"/>
        <rect x="265" y="57" width="110" height="103" rx="3" fill="#abc8d1"/>
        <path d="M320 57v103m-55-52h110" stroke="#6d8e9d" stroke-width="6"/>
        <rect x="24" y="26" width="280" height="34" rx="6" fill="#152634" opacity=".88"/>
        <text x="36" y="49" fill="white" font-family="sans-serif" font-size="17">Canal ${canal} · Imagen de prueba</text>
        <text x="24" y="331" fill="white" font-family="monospace" font-size="15">${new Date().toLocaleTimeString('es-MX')}</text>
      </svg>`)
    }
    if (recurso === 'central') {
      if (metodo === 'PUT') {
        const { url, token } = await recibir(req)
        if (!URL.canParse(url) || !/^https?:\/\//.test(url)) return error(400, 'Escribe una dirección http o https para la central.')
        if (![...vinculos.values()].some(v => v.token === token)) return error(401, 'Usa un token creado en el panel de demostración.')
        central = { url, token, nombre: 'Central de demostración' }
      }
      if (metodo === 'GET' || metodo === 'PUT') return json({ url: central.url, vinculado: Boolean(central.token), nombre: central.nombre })
    }
    if (recurso === 'sucursales-central' && metodo === 'GET') {
      return [...vinculos.values()].some(v => v.token === central.token) ? json(reducidas()) : error(401, 'La central no tiene un vínculo válido.')
    }
    if ((recurso === 'enviar' || recurso === 'exportar') && metodo === 'POST') {
      const datos = await recibir(req)
      const problema = validacion(datos)
      if (problema) return error(400, problema)
      if (recurso === 'exportar') {
        res.setHeader('Content-Disposition', `attachment; filename="sucursal-${datos.id}.json"`)
        return json(datos)
      }
      if (![...vinculos.values()].some(v => v.token === central.token)) return error(401, 'La central no tiene un vínculo válido.')
      guardar(datos, false)
      return json({ ok: true })
    }
  }

  return error(404, 'La ruta no existe en la API falsa.')
}

createServer((req, res) => {
  void atender(req, res).catch(e => {
    const invalido = e instanceof SyntaxError
    res.writeHead(invalido ? 400 : 500, { 'Content-Type': 'application/json; charset=utf-8' })
    res.end(JSON.stringify({ error: invalido ? 'El cuerpo JSON no es válido.' : 'Ocurrió un error en la API falsa.' }))
  })
}).listen(puerto, 'localhost', () => console.log(`API falsa disponible en http://localhost:${puerto}`))
