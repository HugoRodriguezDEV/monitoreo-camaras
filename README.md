# Monitoreo de cámaras

Muestra en una sola pantalla, 24/7, las cámaras en vivo de varias sucursales, aunque cada
una tenga un DVR de marca distinta y esté en otra red. Corre en una PC Linux en modo kiosco.

- Un solo binario de Go (`monitor`) con la interfaz web incluida.
- El video lo mueve [go2rtc](https://github.com/AlexxIT/go2rtc): RTSP, ONVIF y DVR XMEye.
- Marcas: Hikvision (Hik-Connect, Epcom, EZVIZ), Dahua y clones, Uniview, XMEye, ONVIF y RTSP genérico.
- Las sucursales se conectan por [Tailscale](https://tailscale.com): los DVR nunca se exponen a internet.
- Cuadrícula por sucursal, vista general, rotación automática, cámara en grande, aviso de
  sucursal caída y alertas por Telegram.

La arquitectura completa está en [`docs/ARQUITECTURA.md`](docs/ARQUITECTURA.md).

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

## Configuración (`monitor.yaml`)

Todo se configura en un solo archivo: sucursales, DVR (marca, IP, usuario y contraseña),
cámaras por canal, tiempos de rotación, sondeo y alertas de Telegram. El monitor genera de ahí
la configuración de go2rtc y la de la interfaz (`/config.json`, sin contraseñas).

- Ejemplo comentado: [`monitor.example.yaml`](monitor.example.yaml).
- Desarrollo con cámaras de prueba: [`monitor.dev.yaml`](monitor.dev.yaml).
- Marcas soportadas: `hikvision` (Hik-Connect, Epcom, EZVIZ), `dahua` y clones, `uniview`,
  `xmeye` (DVR genéricos), `onvif` y `rtsp` genérico. Detalle en
  [`docs/ARQUITECTURA.md`](docs/ARQUITECTURA.md).
- Antes de instalar, `monitor check -config monitor.yaml` sondea cada DVR e imprime las URLs
  para probarlas en VLC.

## Monitor en Go

El monitor sirve la interfaz y su configuración en el puerto 8080 (8090 en `monitor.dev.yaml`), sondea
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

## Licencia

[MIT](LICENSE).
