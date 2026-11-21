import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// En desarrollo, go2rtc corre aparte (docker-compose.dev.yml) y Vite le pasa
// las peticiones del reproductor y del WebSocket para que todo sea mismo origen.
// En la PC central, go2rtc sirve la interfaz compilada (static_dir) y no hace falta proxy.
const GO2RTC = process.env.GO2RTC_URL ?? 'http://localhost:1984'

export default defineConfig({
  plugins: [react()],
  server: {
    port: 3000,
    proxy: {
      '/api': { target: GO2RTC, ws: true },
      '/video-stream.js': GO2RTC,
      '/video-rtc.js': GO2RTC,
    },
  },
})
