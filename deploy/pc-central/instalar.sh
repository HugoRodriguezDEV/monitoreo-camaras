#!/usr/bin/env bash
# Prepara la PC central (Debian/Ubuntu) — paso 3 del plan.
# Uso:  sudo ./instalar.sh <usuario-kiosco>
# Después: copiar /etc/go2rtc/go2rtc.yaml con los datos reales y `tailscale up`.
set -euo pipefail

KIOSCO="${1:?Uso: sudo ./instalar.sh <usuario-kiosco>}"
GO2RTC_VERSION="${GO2RTC_VERSION:-latest}"
APP_DIR=/opt/monitoreo-camaras

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
id go2rtc >/dev/null 2>&1 || useradd --system --no-create-home --groups video go2rtc
mkdir -p /etc/go2rtc "$APP_DIR"
[ -f /etc/go2rtc/go2rtc.yaml ] || cp "$(dirname "$0")/../../go2rtc/go2rtc.example.yaml" /etc/go2rtc/go2rtc.yaml
chown -R go2rtc: /etc/go2rtc
chmod 600 /etc/go2rtc/go2rtc.yaml
cp "$(dirname "$0")/go2rtc.service" /etc/systemd/system/go2rtc.service
systemctl daemon-reload
systemctl enable --now go2rtc

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
    --autoplay-policy=no-user-gesture-required http://localhost:1984/
fi
PROFILE
chown "$KIOSCO": "$PERFIL"

echo
echo "Listo. Falta:"
echo "  1. Copiar la interfaz compilada (npm run build → dist/) a $APP_DIR/dist"
echo "  2. Llenar /etc/go2rtc/go2rtc.yaml y: systemctl restart go2rtc"
echo "  3. sudo tailscale up --accept-routes"
echo "  4. BIOS: encender al volver la luz (AC power recovery)"
echo "  5. Comprobar: vainfo (debe listar H.264) y chrome://gpu (Video Decode: Hardware accelerated)"
