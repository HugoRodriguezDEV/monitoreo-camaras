import { useEffect, useState } from 'react'
import type { Camara, Config } from '../config'
import { CameraTile } from './CameraTile'

export type Celda = { camara: Camara; sucursal: string; sucursalId: string }

type Props = {
  config: Config
  celdas: Celda[]
  onSeleccionar: (celda: Celda) => void
  detectarPorVideo: boolean
  sucursalesCaidas?: ReadonlySet<string>
}

/** 2×2, 3×3 o 4×4 según el número de cámaras. */
function columnas(n: number): number {
  if (n <= 1) return 1
  if (n <= 4) return 2
  if (n <= 9) return 3
  return 4
}

export function Grid({ config, celdas, onSeleccionar, detectarPorVideo, sucursalesCaidas }: Props) {
  const [enVivo, setEnVivo] = useState<Record<string, boolean>>({})
  const [montado] = useState(() => Date.now())
  const [ahora, setAhora] = useState(() => Date.now())
  const [ultimaImagen, setUltimaImagen] = useState<Record<string, number>>({})

  useEffect(() => {
    const t = setInterval(() => setAhora(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])

  // Sucursales con al menos una cámara en pantalla y ninguna con imagen reciente.
  const limite = config.sinConexionSegundos * 1000
  const sinConexion = new Set<string>()
  const porSucursal = new Map<string, Celda[]>()
  for (const c of celdas) porSucursal.set(c.sucursal, [...(porSucursal.get(c.sucursal) ?? []), c])
  for (const [sucursal, lista] of porSucursal) {
    const alguna = lista.some(
      (c) => enVivo[c.camara.stream] || ahora - (ultimaImagen[c.camara.stream] ?? montado) < limite,
    )
    if (!alguna) sinConexion.add(sucursal)
  }

  const cols = columnas(celdas.length)
  return (
    <div className="grid-wrap">
      <div className="grid" style={{ gridTemplateColumns: `repeat(${cols}, 1fr)` }}>
        {celdas.map((c) => (
          <CameraTile
            key={c.camara.stream}
            config={config}
            stream={c.camara.stream}
            nombre={c.camara.nombre}
            sucursal={c.sucursal}
            sucursalCaida={sucursalesCaidas?.has(c.sucursalId)}
            onSeleccionar={() => onSeleccionar(c)}
            onEnVivo={(v) => {
              setEnVivo((prev) => ({ ...prev, [c.camara.stream]: v }))
              if (!v) setUltimaImagen((prev) => ({ ...prev, [c.camara.stream]: Date.now() }))
            }}
          />
        ))}
      </div>
      {detectarPorVideo && sinConexion.size > 0 && (
        <div className="aviso-desconexion" role="status">
          {[...sinConexion].map((s) => (
            <div key={s}>
              {s} sin conexión <span className="reintentando">· reintentando</span>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
