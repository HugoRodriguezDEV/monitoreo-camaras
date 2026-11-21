#!/usr/bin/env bash
# Prepara la PC central (Debian/Ubuntu) — paso 3 del plan.
# Uso:  sudo ./instalar.sh <usuario-kiosco>
# Después: editar /etc/monitor/monitor.yaml y ejecutar `tailscale up`.
set -euo pipefail

KIOSCO="${1:?Uso: sudo ./instalar.sh <usuario-kiosco>}"
GO2RTC_VERSION="${GO2RTC_VERSION:-latest}"
SCRIPT_DIR="$(cd -- "$(dirname -- "$0")" && pwd)"
if [ ! -x "$SCRIPT_DIR/monitor" ]; then
  echo "Falta el binario ejecutable monitor junto a instalar.sh" >&2
  exit 1
fi

echo "== Paquetes"
apt-get update
apt-get install -y intel-media-va-driver vainfo cage chromium curl ca-certificates

echo "== Tailscale"
command -v tailscale >/dev/null || curl -fsSL https://tailscale.com/install.sh | sh

echo "== go2rtc"
if [ "$GO2RTC_VERSION" = latest ]; then
  URL=https://github.com/AlexxIT/go2rtc/releases/latest/download/go2rtc_linux_amd64
else
  URL="https://github.com/AlexxIT/go2rtc/releases/download/${GO2RTC_VERSION}/go2rtc_linux_amd64"
fi
curl -fsSL "$URL" -o /usr/local/bin/go2rtc
chmod +x /usr/local/bin/go2rtc
echo "== Monitor"
if systemctl cat go2rtc.service >/dev/null 2>&1; then
  systemctl disable --now go2rtc.service
fi
id monitor >/dev/null 2>&1 || useradd --system --user-group --no-create-home monitor
install -d -o monitor -g monitor -m 0750 /var/lib/monitor
install -d -o root -g monitor -m 0750 /etc/monitor
install -m 0755 "$SCRIPT_DIR/monitor" /usr/local/bin/monitor
if [ ! -f /etc/monitor/monitor.yaml ]; then
  install -o monitor -g monitor -m 0600 "$SCRIPT_DIR/../../monitor.example.yaml" /etc/monitor/monitor.yaml
fi
chown monitor:monitor /etc/monitor/monitor.yaml
chmod 0600 /etc/monitor/monitor.yaml
install -m 0644 "$SCRIPT_DIR/monitor.service" /etc/systemd/system/monitor.service
systemctl daemon-reload
systemctl enable monitor.service
systemctl restart monitor.service

echo "== Sin suspensión"
systemctl mask sleep.target suspend.target hibernate.target hybrid-sleep.target

echo "== Kiosco: inicio automático de $KIOSCO en tty1 con cage + chromium"
id "$KIOSCO" >/dev/null 2>&1 || useradd -m -G video,render "$KIOSCO"
mkdir -p /etc/systemd/system/getty@tty1.service.d
cat > /etc/systemd/system/getty@tty1.service.d/autologin.conf <<CONF
[Service]
ExecStart=
ExecStart=-/sbin/agetty --autologin $KIOSCO --noclear %I \$TERM
CONF
PERFIL="/home/$KIOSCO/.bash_profile"
grep -q monitoreo-camaras "$PERFIL" 2>/dev/null || cat >> "$PERFIL" <<'PROFILE'
# monitoreo-camaras: kiosco en tty1
if [ "$(tty)" = /dev/tty1 ]; then
  exec cage -- chromium --kiosk --ignore-gpu-blocklist --noerrdialogs --disable-session-crashed-bubble \
    --autoplay-policy=no-user-gesture-required http://localhost:8080/
fi
PROFILE
# Actualiza el bloque de kiosco de instalaciones anteriores.
sed -i '/# monitoreo-camaras: kiosco en tty1/,/^fi$/s|http://localhost:1984/|http://localhost:8080/|' "$PERFIL"
chown "$KIOSCO": "$PERFIL"

echo
echo "Listo. Falta:"
echo "  1. Llenar /etc/monitor/monitor.yaml con los DVR reales"
echo "  2. Ejecutar sudo systemctl restart monitor"
echo "  3. sudo tailscale up --accept-routes"
echo "  4. BIOS: encender al volver la luz (AC power recovery)"
echo "  5. Comprobar: vainfo (debe listar H.264) y chrome://gpu (Video Decode: Hardware accelerated)"
