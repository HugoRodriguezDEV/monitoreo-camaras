# Arquitectura — monitor de cámaras

Un solo binario de Go, `monitor`, corre en la PC central (Linux). Hace todo lo que no es
video y supervisa a go2rtc, que es quien mueve el video.

```
                        PC central (Linux, Chromium en kiosco → http://localhost:8080)
                        ┌────────────────────────────────────────────────────────────┐
DVR suc. 1 ─┐           │  monitor (Go)  :8080                                       │
DVR suc. 2 ─┼─Tailscale─┤   ├─ lee monitor.yaml                                      │
DVR suc. 3 ─┤           │   ├─ genera go2rtc.yaml y supervisa go2rtc (127.0.0.1:1984)│
DVR suc. 4 ─┘           │   ├─ vigila cada DVR (sondeo TCP) → /api/estado            │
 (4 marcas distintas)   │   ├─ sirve la interfaz web (embebida con go:embed)         │
                        │   ├─ proxy /api/ws y /video-stream.js → go2rtc             │
                        │   └─ avisa por Telegram cuando una sucursal cae o vuelve   │
                        └────────────────────────────────────────────────────────────┘
```

**Por qué go2rtc y no video propio:** los 4 DVR son de marcas distintas. go2rtc ya habla RTSP
(Hikvision, Dahua, Uniview…), ONVIF y el protocolo propio de los DVR XMEye (`dvrip`). El
`monitor` solo traduce "marca + IP + canal" a la URL que go2rtc entiende.

## Estructura del repo

```
cmd/monitor/            main: subcomandos run | check | go2rtc
internal/config/        tipos y carga de monitor.yaml, validación
internal/marcas/        "marca + DVR + canal + calidad" → URL de go2rtc
internal/go2rtc/        genera go2rtc.yaml y supervisa el proceso
internal/salud/         sondeo periódico de cada DVR, estado por sucursal
internal/alertas/       Telegram (opcional)
internal/servidor/      HTTP: interfaz embebida, /config.json, /api/estado, proxy a go2rtc
web/                    interfaz React (Vite). `npm run build` → web/dist, que embebe el binario
deploy/                 systemd, instalación de la PC central, go2rtc de desarrollo
monitor.example.yaml    configuración de ejemplo (sin contraseñas reales)
```

`internal/servidor` embebe `web/dist` con `//go:embed`. Para que `go build` funcione sin
haber compilado la web, el paquete `web` (archivo `web/embed.go`) expone `web.Dist fs.FS` y el
repo incluye `web/dist/.gitkeep`; si `index.html` no existe, el servidor responde una página
que dice "Interfaz no compilada: corre `npm run build` en web/".

## monitor.yaml (contrato)

```yaml
servidor:
  escuchar: ":8080"

go2rtc:
  binario: /usr/local/bin/go2rtc   # ruta del ejecutable que supervisa el monitor
  api: "127.0.0.1:1984"            # donde escucha go2rtc (solo local)
  config: /var/lib/monitor/go2rtc.yaml   # archivo que genera el monitor
  externo: false                   # true = no lo arranca, usa uno que ya corre (desarrollo)

interfaz:
  rotacion_segundos: 25
  rotar_vista_general: true
  volver_a_cuadricula_segundos: 60

salud:
  intervalo_segundos: 10
  caido_tras_fallos: 3             # sondeos fallidos seguidos para marcar la sucursal caída

alertas:
  telegram:
    token: ""                      # vacío = sin alertas
    chat_id: ""

sucursales:
  - id: s1
    nombre: Sucursal Centro
    dvr:
      marca: hikvision             # hikvision | dahua | xmeye | uniview | onvif | rtsp
      host: 192.168.1.64
      puerto: 0                    # 0 = el de la marca (554 RTSP, 34567 xmeye, 80 onvif)
      usuario: admin
      password: CAMBIAR
      # solo para marca: rtsp → plantillas con {canal}
      ruta_sub: ""                 # ej. "/live/ch{canal}/sub"
      ruta_principal: ""
    camaras:
      - canal: 1
        nombre: Entrada
        clave: true                # sale en la vista general
      - canal: 2
        nombre: Caja
```

Validación: ids de sucursal únicos y `[a-z0-9_]+`; al menos una sucursal y una cámara por
sucursal; marca conocida; `rtsp` exige `ruta_sub`; canales ≥ 1 y sin repetir por sucursal.
Los errores dicen qué sucursal y qué campo.

## Marcas → URL de go2rtc

Nombre del stream en go2rtc: `{sucursal}_c{canal}` (substream) y `{sucursal}_c{canal}_hd`
(principal). Usuario y contraseña van escapados para URL.

| marca | substream | principal |
|---|---|---|
| hikvision (Hik-Connect, Epcom, EZVIZ) | `rtsp://U:P@H:554/Streaming/Channels/{canal}02` | `…/{canal}01` |
| dahua (y clones: Amcrest, Lorex, Imou) | `rtsp://U:P@H:554/cam/realmonitor?channel={canal}&subtype=1` | `…subtype=0` |
| uniview | `rtsp://U:P@H:554/unicast/c{canal}/s1/live` | `…/s0/live` |
| xmeye (Xiongmai, DVR genéricos) | `dvrip://U:P@H:34567?channel={canal-1}&subtype=1` | `…subtype=0` |
| onvif | `onvif://U:P@H:80?subtype=1` (canal → perfil, ver nota) | `…subtype=0` |
| rtsp (genérico) | `rtsp://U:P@H:554` + `ruta_sub` con `{canal}` reemplazado | `ruta_principal` o, si falta, la misma que sub |

Nota ONVIF: go2rtc elige el perfil con `subtype`; para DVR multicanal por ONVIF se documenta
que conviene usar `rtsp` genérico con la ruta que reporte ONVIF Device Manager.

## HTTP del monitor (contrato con la interfaz)

| Ruta | Respuesta |
|---|---|
| `GET /` y estáticos | interfaz embebida (`web/dist`), con fallback a `index.html` |
| `GET /config.json` | configuración para la interfaz, **sin credenciales** (forma abajo) |
| `GET /api/estado` | estado de salud (forma abajo) |
| `GET /api/ws?src=…`, `/video-stream.js`, `/video-rtc.js` | proxy a go2rtc (WebSocket incluido) |
| `GET /salud` | `200 ok` si el monitor vive (para systemd/monitoreo) |

Cualquier otra ruta `/api/*` de go2rtc **no** se expone (el panel de go2rtc trae las URLs con
contraseñas).

`/config.json`:

```json
{
  "go2rtc": "",
  "rotacionSegundos": 25,
  "rotarVistaGeneral": true,
  "volverACuadriculaSegundos": 60,
  "sinConexionSegundos": 30,
  "sucursales": [
    {
      "id": "s1",
      "nombre": "Sucursal Centro",
      "camaras": [
        { "stream": "s1_c1", "streamHd": "s1_c1_hd", "nombre": "Entrada", "clave": true }
      ]
    }
  ]
}
```

`sinConexionSegundos` = `intervalo_segundos × caido_tras_fallos`.

`/api/estado`:

```json
{
  "actualizado": "2026-11-21T19:00:00Z",
  "go2rtc": { "enLinea": true },
  "sucursales": [
    { "id": "s1", "enLinea": true, "desde": "2026-11-21T18:40:00Z", "latenciaMs": 42, "error": "" }
  ]
}
```

La interfaz consulta `/api/estado` cada 5 s. Una sucursal con `enLinea: false` muestra el aviso
"{nombre} sin conexión · reintentando" en su vista y en la vista general. Si `/api/estado` no
responde (modo desarrollo sin monitor), la interfaz conserva la detección actual por video.

## Salud

Cada `intervalo_segundos`, sondeo TCP al puerto del DVR (timeout 3 s). Tras `caido_tras_fallos`
fallos seguidos la sucursal pasa a caída; un sondeo exitoso la regresa. Cada cambio de estado se
registra en el log y, si hay Telegram, manda "🔴 Sucursal Centro sin conexión" / "🟢 Sucursal
Centro de vuelta (caída 12 min)". go2rtc también se vigila: si su API no responde, `go2rtc.enLinea`
es false.

## Supervisión de go2rtc

Al arrancar `monitor run`: genera el `go2rtc.yaml` (permisos 0600, `api.listen` en
`go2rtc.api`, `api.origin` vacío, todos los streams), lanza go2rtc con `-config <archivo>`,
reenvía su salida al log con prefijo `[go2rtc]`, y si muere lo reinicia con espera creciente
(1 s, 2 s, 4 s… máx 30 s; se reinicia el contador tras 60 s estable). Al recibir SIGINT/SIGTERM
detiene go2rtc y sale limpio. Con `externo: true` no lanza nada.

## Subcomandos

- `monitor run [-config monitor.yaml]` — todo lo anterior.
- `monitor check [-config …]` — valida la config, sondea cada DVR una vez e imprime por
  sucursal: marca, alcanzable sí/no, y las URLs de sub y principal **con la contraseña
  enmascarada**, listas para probar en VLC (opción `-mostrar-password` para verlas completas).
- `monitor go2rtc [-config …]` — imprime el `go2rtc.yaml` generado (para depurar).

## Despliegue

`GOOS=linux GOARCH=amd64 go build -o monitor ./cmd/monitor` después de `npm run build` en `web/`.
`deploy/pc-central/instalar.sh` instala go2rtc + `monitor` como servicio systemd
(`deploy/pc-central/monitor.service`), y el kiosco abre `http://localhost:8080/`.
