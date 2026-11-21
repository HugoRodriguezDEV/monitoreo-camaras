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
| `monitor.example.yaml` | Configuración de ejemplo del monitor y sus grabadores |
| `go2rtc/go2rtc.dev.yaml` | go2rtc con cámaras de prueba para desarrollar sin grabadores |
| `deploy/pc-central/` | Instalador y servicio systemd del monitor |

## Desarrollo (sin cámaras reales)

```bash
npm install
docker compose -f docker-compose.dev.yml up -d   # go2rtc con 10 cámaras de prueba
npm run dev                                      # http://localhost:3000
```

`s4_cam1` apunta a una IP que no existe, a propósito, para ver el aviso de
"Sucursal 4 sin conexión". El panel de go2rtc queda en http://localhost:1984.

Para apagar go2rtc: `docker compose -f docker-compose.dev.yml down`.

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

## Monitor en Go

El monitor sirve la interfaz y su configuración en el puerto 8080, sondea
los grabadores y supervisa go2rtc. El contrato está en
[`docs/ARQUITECTURA.md`](docs/ARQUITECTURA.md).

Con el go2rtc de Docker en ejecución, usa estos comandos desde la raíz:

```bash
go run ./cmd/monitor check -config monitor.dev.yaml
go run ./cmd/monitor run -config monitor.dev.yaml
```

`check` muestra la conectividad de las cuatro sucursales y enmascara las
contraseñas. La IP de la cuarta sucursal está configurada para simular una caída.
En modo externo, el monitor usa el go2rtc que ya está escuchando en el puerto 1984.

## Producción (PC central)

1. Ejecutar `make build` para compilar la interfaz y generar `dist/monitor` para Linux amd64.
2. En la PC central, colocar el binario junto a `deploy/pc-central/instalar.sh`
   con el nombre `monitor` y ejecutar `sudo deploy/pc-central/instalar.sh kiosco`.
3. Editar `/etc/monitor/monitor.yaml` con los datos reales y ejecutar
   `sudo systemctl restart monitor`.
4. Abrir `http://localhost:8080/` para comprobar el kiosco.

## Pendiente (v2 según el plan)

- Control con teclado o control remoto (flechas para cambiar de sucursal).
- Acceso desde el celular de los dueños por la VPN.
