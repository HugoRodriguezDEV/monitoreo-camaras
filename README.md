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

Para probar el panel y el asistente sin Go ni go2rtc, ejecuta `npm run dev:falso`
en `web/`. Abre http://localhost:3000/admin o http://localhost:3000/instalar.
El DVR de prueba usa el usuario `admin` y la contraseña `demo`. Para probar el
envío, crea un vínculo en el panel y usa su token y `http://localhost:3000`
en el asistente. Los cambios se pierden al detener el servidor falso.

## Configuración (`monitor.yaml`)

En producción, `monitor.yaml` configura el servidor, go2rtc, el sondeo y el directorio
`datos`. El valor predeterminado de `datos` es `/var/lib/monitor` en Linux y
`%APPDATA%\monitor-camaras` en Windows. Consulta [monitor.example.yaml](monitor.example.yaml).

La primera vez, el monitor importa las sucursales, la interfaz y las alertas del YAML
al almacén. Una central puede empezar vacía. Después puedes editar las sucursales, las
cámaras, la rotación y Telegram desde `/admin`. El panel también permite identificar
y reiniciar grabadores compatibles y crear o revocar vínculos para el instalador.

- **`sucursales.json` es sensible**. Contiene las contraseñas de los DVR en claro y el
  monitor lo escribe con permisos `0600` en Unix. Protege también el `go2rtc.yaml`
  generado, las exportaciones de sucursales y cualquier respaldo de estos archivos.
- `ajustes.json` contiene la configuración de la interfaz y las credenciales de Telegram.
  `admin.json` contiene el hash bcrypt de la contraseña de administración y los hashes
  SHA-256 de los tokens de vínculo. Guarda cada token al crearlo, porque se muestra una vez.

## Monitor en Go

`monitor run -config monitor.yaml` sirve el kiosco y el panel, vigila los grabadores
y supervisa go2rtc. Los cambios de sucursales o ajustes actualizan la configuración
pública y la vigilancia, aumentan `configVersion` y reinician el go2rtc supervisado sin
detener el monitor. Si un DVR identificado cambia de IP, el monitor busca su MAC o
serie en el mismo segmento /24 después de tres minutos de caída por defecto y avisa
por Telegram cuando encuentra la nueva dirección. El contrato está en [docs/V2.md](docs/V2.md).

Desde la raíz, con el go2rtc de Docker en ejecución, puedes probar el backend así.

```bash
go run ./cmd/monitor run -config monitor.dev.yaml
go run ./cmd/monitor check -config monitor.dev.yaml
```

El monitor de desarrollo escucha en el puerto 8090 y guarda los cambios en `./.datos-dev`.
`check` lee las sucursales del almacén, sondea cada DVR y enmascara las contraseñas de
las URLs de prueba. El modo externo utiliza el go2rtc que ya escucha en el puerto 1984.

En la laptop de la sucursal, ejecuta `monitor instalador` para abrir el asistente local
en el navegador. Vincúlalo con la URL de la central y un token del panel para enviar
sucursales. El asistente también puede exportarlas a un archivo cuando no hay conexión.
Puedes usar `-datos <directorio>` para cambiar dónde guarda el vínculo y `-no-abrir` para
abrir manualmente la URL que imprime.

## App de instalación (Windows)

1. Abre la sección **Assets** de la última publicación y descarga **MonitorCamaras-Instalador.exe**. La versión portable se puede abrir desde Descargas. Si prefieres instalarla y tener un acceso en el menú Inicio, descarga **MonitorCamaras-Instalador-Setup.exe**. [Publicaciones del proyecto](https://github.com/HugoRodriguezDEV/monitoreo-camaras/releases/latest)
2. Haz doble clic en el archivo. Si Windows muestra el aviso de SmartScreen porque el ejecutable aún no está firmado, selecciona **Más información → Ejecutar de todas formas** después de comprobar que lo descargaste desde la publicación del proyecto.
3. Se abrirá la ventana **Monitor de cámaras — Instalador**. Si te pide instalar Microsoft Edge WebView2, acepta para que Windows pueda mostrar el asistente.

Estos son los pasos que verás en la ventana.

1. **Central.** Escribe la dirección de la computadora que muestra las cámaras y el token que genera su panel en **Vínculos**. Si trabajas sin conexión a la central, elige **Sin conexión, solo exportar**.
2. **Buscar.** Conecta la computadora a la misma red que el grabador (DVR). Pulsa **Buscar DVR** y elige el equipo encontrado. Si ya conoces su dirección, puedes agregarlo a mano.
3. **Acceso.** Escribe el usuario y la contraseña del grabador y pulsa **Identificar y ver cámaras**.
4. **Revisión y vista previa.** Revisa las imágenes, nombra las cámaras e indica cuáles deben aparecer en la vista general de la tele. Agrega los canales manualmente si el DVR no los informa.
5. **Enviar.** Indica el nombre y el identificador de la sucursal. Envía la configuración a la central o guárdala con **Exportar archivo**. El archivo contiene la contraseña del DVR, así que guárdalo en un lugar seguro.

### Compilar el asistente en Windows

Desde la raíz del repositorio, en PowerShell:

```powershell
npm ci --prefix web
npm run build --prefix web
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
go build -tags desktop,production -ldflags '-H windowsgui' -o dist/MonitorCamaras-Instalador.exe ./cmd/instalador
```

Para generar también el instalador de menú Inicio, agrega `makensis.exe` de NSIS al PATH y ejecuta:

```powershell
makensis.exe /WX /INPUTCHARSET UTF8 "/DPORTABLE=$PWD\dist\MonitorCamaras-Instalador.exe" "/DSALIDA=$PWD\dist\MonitorCamaras-Instalador-Setup.exe" cmd/instalador/build/windows/instalador.nsi
```

Si cambias el ícono, vuelve a exportar el SVG a PNG de 1024×1024 y regenera el recurso de Windows. El archivo generado queda junto al código de la app para que `go build` pueda incorporarlo.

```powershell
go run github.com/tc-hib/go-winres@v0.3.3 make --in cmd/instalador/build/windows/recursos.json --arch amd64 --out cmd/instalador/recursos
```

## Producción (PC central)

1. Ejecutar `make build` para compilar la interfaz y generar `dist/monitor` para Linux amd64.
2. En la PC central, colocar el binario junto a `deploy/pc-central/instalar.sh`
   con el nombre `monitor` y ejecutar `sudo deploy/pc-central/instalar.sh kiosco`.
3. Ejecutar `sudo tailscale up --accept-routes` y abrir `http://localhost:8080/admin`
   para configurar la contraseña de administración y agregar las sucursales.
4. Abrir `http://localhost:8080/` para comprobar el kiosco.

## Pendiente

- Control con teclado o control remoto (flechas para cambiar de sucursal).
- Acceso desde el celular de los dueños por la VPN.

## Licencia

[MIT](LICENSE).
