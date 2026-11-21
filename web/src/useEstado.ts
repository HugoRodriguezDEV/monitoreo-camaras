import { useEffect, useState } from 'react'

type Estado = {
  actualizado: string
  go2rtc: { enLinea: boolean }
  sucursales: { id: string; enLinea: boolean; desde: string; latenciaMs: number; error: string }[]
}

export function useEstado() {
  const [estado, setEstado] = useState<Estado | 'desconocido'>('desconocido')

  useEffect(() => {
    const desmontaje = new AbortController()

    async function consultar() {
      try {
        const res = await fetch('/api/estado', {
          cache: 'no-store',
          signal: AbortSignal.any([desmontaje.signal, AbortSignal.timeout(5000)]),
        })
        if (!res.ok) throw new Error(`No se pudo consultar el estado (${res.status})`)
        const datos = (await res.json()) as Estado
        if (!desmontaje.signal.aborted) setEstado(datos)
      } catch {
        if (!desmontaje.signal.aborted) setEstado('desconocido')
      }
    }

    void consultar()
    const intervalo = setInterval(consultar, 5000)
    return () => {
      clearInterval(intervalo)
      desmontaje.abort()
    }
  }, [])

  return estado
}
