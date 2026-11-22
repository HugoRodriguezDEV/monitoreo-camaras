import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// El monitor sirve la configuración, el estado y el reproductor bajo el mismo origen.
const MONITOR = process.env.MONITOR_URL ?? 'http://localhost:8090'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/config.json': MONITOR,
      '/api/estado': MONITOR,
      '/api/admin': MONITOR,
      '/api/vinculo': MONITOR,
      '/api/instalador': MONITOR,
      '/api/ws': { target: MONITOR, ws: true },
      '/video-stream.js': MONITOR,
      '/video-rtc.js': MONITOR,
    },
  },
})
