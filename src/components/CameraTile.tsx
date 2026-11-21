import { useEffect, useRef, useState } from 'react'
import { urlStream, type Config } from '../config'

type Props = {
  config: Config
  stream: string
  nombre: string
  sucursal: string
  onSeleccionar?: () => void
  /** Avisa cada vez que la cámara pasa de tener imagen a no tenerla, o al revés. */
  onEnVivo?: (enVivo: boolean) => void
}

// El elemento <video-stream> de go2rtc se reconecta solo; aquí solo vigilamos
// si el video avanza para saber si hay imagen.
export function CameraTile({ config, stream, nombre, sucursal, onSeleccionar, onEnVivo }: Props) {
  const host = useRef<HTMLDivElement>(null)
  const [enVivo, setEnVivo] = useState(false)
  const onEnVivoRef = useRef(onEnVivo)
  useEffect(() => {
    onEnVivoRef.current = onEnVivo
  })

  useEffect(() => {
    const el = document.createElement('video-stream') as HTMLElement & { src: string }
    host.current?.appendChild(el)
    el.src = urlStream(config, stream)

    let ultimoTiempo = -1
    let ultimoAvance = 0
    let estado = false
    const timer = setInterval(() => {
      const video = el.querySelector('video')
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
        <span className="tile-sucursal">{sucursal}</span>
      </div>
    </div>
  )
}
