import { useEffect, useRef, useState } from 'react'
import { urlStream, type Config } from '../config'

type Props = {
  config: Config
  stream: string
  nombre: string
  sucursal: string
  sucursalCaida?: boolean
  onSeleccionar?: () => void
  /** Avisa cada vez que la cámara pasa de tener imagen a no tenerla, o al revés. */
  onEnVivo?: (enVivo: boolean) => void
}

/** Sin avance del video durante este tiempo, se reconecta la cámara. */
const REINTENTO_MS = 10_000

// El elemento <video-stream> de go2rtc se reconecta solo cuando se cierra el
// WebSocket, pero no cuando el socket queda abierto sin video (pasa, por ejemplo,
// si la misma cámara se desconecta y se vuelve a conectar al rotar). Por eso aquí
// se vigila que el video avance y, si se atora, se recrea el reproductor.
export function CameraTile({ config, stream, nombre, sucursal, sucursalCaida, onSeleccionar, onEnVivo }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const [enVivo, setEnVivo] = useState(false)
  const onEnVivoRef = useRef(onEnVivo)
  useEffect(() => {
    onEnVivoRef.current = onEnVivo
  })

  useEffect(() => {
    let el: HTMLElement & { src: string }
    let ultimoTiempo = -1
    let ultimoAvance = 0
    function conectar() {
      el = document.createElement('video-stream') as HTMLElement & { src: string }
      host.current?.appendChild(el)
      el.src = urlStream(config, stream)
      ultimoTiempo = -1
      ultimoAvance = Date.now()
    }
    conectar()

    let estado = false
    const timer = setInterval(() => {
      if (Date.now() - ultimoAvance > REINTENTO_MS) {
        el.remove()
        conectar()
      }
      const video = el.querySelector('video')
      // En la tele no se usan los controles del navegador.
      if (video?.controls) video.controls = false
      const t = video?.currentTime ?? -1
      if (t !== ultimoTiempo) {
        ultimoTiempo = t
        ultimoAvance = Date.now()
      }
      const ahora = t > 0 && Date.now() - ultimoAvance < 5000
      if (ahora !== estado) {
        estado = ahora
        setEnVivo(ahora)
        onEnVivoRef.current?.(ahora)
      }
    }, 1000)

    return () => {
      clearInterval(timer)
      el.remove()
    }
  }, [config, stream])

  return (
    <div className="tile" onClick={onSeleccionar}>
      <div className="tile-video" ref={host} />
      {!enVivo && <div className="tile-espera">Conectando…</div>}
      <div className="tile-etiqueta">
        <span className="tile-nombre">{nombre}</span>
        <span className="tile-sucursal">
          {sucursalCaida && <span className="punto-caida" role="img" aria-label="Sin conexión" />}
          {sucursal}
        </span>
      </div>
    </div>
  )
}
