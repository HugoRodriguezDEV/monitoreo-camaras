import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'

const puerto = process.env.API_FALSA_PUERTO ?? '8090'
const entorno = { ...process.env, MONITOR_URL: `http://localhost:${puerto}` }
const archivo = ruta => fileURLToPath(new URL(ruta, import.meta.url))
const procesos = [
  spawn(process.execPath, [archivo('./api-falsa.mjs')], { stdio: 'inherit', env: entorno }),
  spawn(process.execPath, [archivo('../node_modules/vite/bin/vite.js'), ...process.argv.slice(2)], { stdio: 'inherit', env: entorno }),
]

function detener(codigo) {
  for (const proceso of procesos) proceso.kill()
  process.exitCode = codigo
}

for (const proceso of procesos) {
  proceso.on('exit', codigo => detener(process.exitCode ?? codigo ?? 1))
  proceso.on('error', error => {
    console.error('No se pudo iniciar el entorno de prueba.', error.message)
    detener(1)
  })
}
process.on('SIGINT', () => detener(0))
process.on('SIGTERM', () => detener(0))
