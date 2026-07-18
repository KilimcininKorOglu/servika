#!/usr/bin/env bash
# servika-redis-setup installs isolated per-tenant Redis infrastructure using Valkey.
# It is idempotent, runs during installation, and supports the panel's Redis Cache feature.
set -uo pipefail
log(){ printf '  %s\n' "$*"; }

echo "════ Valkey + php-redis installation ════"
PHP_REDIS_PKGS=""
for v in php74 php80 php81 php82 php83 php84 php85; do
  [ -d "/etc/opt/remi/$v" ] && PHP_REDIS_PKGS="$PHP_REDIS_PKGS ${v}-php-pecl-redis6"
done
dnf install -y valkey php-pecl-redis6 $PHP_REDIS_PKGS >/tmp/redis-setup.log 2>&1 \
  && log "valkey + php-redis installed" || { log "installation warning (some packages may already be installed)"; }

echo "════ Administrator password + ACL file ════"
ENV=/etc/servika/env
ADMIN=$(grep -oP '^SERVIKA_REDIS_ADMIN_PASS=\K.*' "$ENV" 2>/dev/null)
if [ -z "$ADMIN" ]; then
  ADMIN=$(openssl rand -hex 24)
  echo "SERVIKA_REDIS_ADMIN_PASS=${ADMIN}" >> "$ENV"
  log "administrator password generated and added to the environment file"
fi
# Add the default administrator entry when absent while preserving tenant ACLs.
ACLF=/etc/valkey/users.acl
if [ ! -f "$ACLF" ] || ! grep -q '^user default ' "$ACLF"; then
  printf 'user default on >%s ~* &* +@all\n' "$ADMIN" > "$ACLF"
  log "users.acl created"
fi
chown valkey:valkey "$ACLF" 2>/dev/null; chmod 640 "$ACLF"

echo "════ valkey.conf cache tuning ════"
CONF=/etc/valkey/valkey.conf
if ! grep -q 'servika-cache' "$CONF"; then
cat >> "$CONF" <<VK

# ===== servika-cache =====
maxmemory 256mb
maxmemory-policy allkeys-lru
save ""
appendonly no
aclfile /etc/valkey/users.acl
VK
  log "valkey.conf tuning added"
fi
sed -i '/^requirepass /d' "$CONF"   # requirepass conflicts with aclfile.

echo "════ SELinux (php-fpm → redis TCP) ════"
setsebool -P httpd_can_network_connect 1 2>/dev/null && log "httpd_can_network_connect=1"

echo "════ valkey enable + (re)start ════"
systemctl enable valkey >/dev/null 2>&1
systemctl restart valkey; sleep 2
if systemctl is-active --quiet valkey && REDISCLI_AUTH="$ADMIN" valkey-cli PING 2>/dev/null | grep -q PONG; then
  log "✓ valkey ACTIVE + admin auth OK"
else
  log "✗ valkey could not be started; journalctl -u valkey"
  exit 1
fi
echo "════════ ✓ Redis infrastructure ready ════════"
