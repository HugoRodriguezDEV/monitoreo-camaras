# Monitoreo de cámaras

Una sola pantalla con las cámaras en vivo de las 4 sucursales, 24/7, desde una PC
Linux. El plan completo está en [`docs/PLAN.md`](docs/PLAN.md).

```
Grabador (RTSP) ──Tailscale──► go2rtc (PC central) ──WebRTC/MSE──► esta interfaz (Chromium kiosco)
```

## Estructura

| Ruta | Qué es |
|---|---|
| `src/` | Interfaz React: cuadrícula por sucursal, vista general, rotación, cámara en grande, aviso de sucursal sin conexión, reloj |
| `public/config.json` | Sucursales, cámaras y tiempos. Se edita sin recompilar |
| `go2rtc/go2rtc.example.yaml` | Plantilla de go2rtc para la PC central (rutas RTSP de Hikvision/Epcom/EZVIZ) |
| `go2rtc/go2rtc.dev.yaml` | go2rtc con cámaras de prueba para desarrollar sin grabadores |
| `infra/pc-central/` | Script de instalación de la PC central y servicio systemd de go2rtc |

## Desarrollo (sin cámaras reales)

Arranca go2rtc desde la raíz del repositorio.

```bash
docker compose -f docker-compose.dev.yml up -d
```

Arranca el monitor en una terminal.

```bash
go run ./cmd/monitor run -config monitor.dev.yaml
```

Instala las dependencias de la interfaz con `npm ci` en `web/` la primera vez.
Abre otra terminal desde la raíz y ejecuta:

```bash
cd web && npm run dev
```

Abre http://localhost:3000. La cámara `s4_c1` apunta a una IP inalcanzable
para probar el aviso de desconexión. Para detener go2rtc, ejecuta
`docker compose -f docker-compose.dev.yml down`.

## `config.json`

| Clave | Default | Qué hace |
|---|---|---|
| `go2rtc` | `""` | Base de go2rtc. Vacío = mismo origen |
| `rotacionSegundos` | 25 | Cada cuánto cambia de vista |
| `rotarVistaGeneral` | true | La vista general entra en la rotación |
| `volverACuadriculaSegundos` | 60 | La cámara en grande vuelve sola a la cuadrícula |
| `sinConexionSegundos` | 15 | Sin imagen en todas las cámaras de una sucursal → aviso |
| `sucursales[].camaras[]` | — | `stream` (substream en go2rtc), `streamHd` (principal, opcional), `nombre`, `clave` (sale en la vista general) |

Los nombres de `stream` deben coincidir con los de `streams:` en el `go2rtc.yaml`.

## Producción (PC central)

1. `npm run build` y copiar `dist/` a `/opt/monitoreo-camaras/dist` en la PC.
2. En la PC: `sudo infra/pc-central/instalar.sh kiosco`.
3. Llenar `/etc/go2rtc/go2rtc.yaml` con los datos del levantamiento. Ese archivo lleva
   contraseñas: **no se sube a git** (`go2rtc/go2rtc.yaml` está en `.gitignore`).
4. go2rtc sirve la interfaz en `http://localhost:1984/` (`static_dir`), así que no hace
   falta otro servidor web; el kiosco abre esa dirección.

## Pendiente (v2 según el plan)

- Control con teclado o control remoto (flechas para cambiar de sucursal).
- Acceso desde el celular de los dueños por la VPN.
