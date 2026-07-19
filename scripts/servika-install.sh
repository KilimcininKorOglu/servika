#!/usr/bin/env bash
# servika-install turns a clean AlmaLinux 10 server into a complete Servika installation.
# It is idempotent and must run as root.
#
#   ./servika-install.sh [--admin-password <password>] [--admin-email <email>]
#
# The assets directory must be located next to this script:
#   servika-server  servika-seed-admin  frontend-dist.tar.gz
#   migrations.tar.gz  nginx/*  php-fpm/*  phpmyadmin/*  systemd/*  ops/*
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
A="$HERE/assets"
ADMIN_PASSWORD=""; ADMIN_EMAIL="admin@local"
while [ $# -gt 0 ]; do case "$1" in
  --admin-password) shift; ADMIN_PASSWORD="$1" ;;
  --admin-email) shift; ADMIN_EMAIL="$1" ;;
  *) echo "unknown option: $1"; exit 2 ;;
esac; shift; done

c_g="\033[32m"; c_y="\033[33m"; c_r="\033[31m"; c_b="\033[1;34m"; c_0="\033[0m"
[ -t 1 ] || { c_g=; c_y=; c_r=; c_b=; c_0=; }
step(){ echo -e "\n${c_b}══ $* ══${c_0}"; }
ok(){ echo -e "  ${c_g}✓${c_0} $*"; }
warn(){ echo -e "  ${c_y}!${c_0} $*"; }
die(){ echo -e "  ${c_r}✗ $*${c_0}"; exit 1; }

[ "$(id -u)" = 0 ] || die "root is required"
[ -d "$A" ] || die "assets/ was not found ($A)"
grep -qiE "AlmaLinux|Rocky|Red Hat|CentOS" /etc/os-release || warn "AlmaLinux/RHEL 10 was expected, continuing anyway"

PHP_VERS="74 80 81 82 83 84 85 86"
PHP_EXT="fpm cli mysqlnd mbstring bcmath intl gd soap opcache pdo xml zip pgsql ldap"

# ============ 1) REPOSITORIES ============
step "1) Repositories (EPEL + Remi + CRB)"
dnf install -y epel-release >/dev/null 2>&1 && ok "EPEL"
rpm -q remi-release >/dev/null 2>&1 || dnf install -y https://rpms.remirepo.net/enterprise/remi-release-10.rpm >/dev/null 2>&1
rpm -q remi-release >/dev/null 2>&1 && ok "Remi" || die "Remi could not be added"
dnf config-manager --set-enabled crb >/dev/null 2>&1 && ok "CRB"

# ============ 2) BASE PACKAGES ============
step "2) Base packages"
dnf install -y nginx httpd mariadb-server valkey certbot python3-certbot-nginx \
  clamav clamav-update httpd-tools mod_proxy_html tar openssl policycoreutils-python-utils \
  setools-console jq bind bind-utils nftables unzip zip cronie xfsprogs sudo \
  acl libarchive bubblewrap rsync git curl >/dev/null 2>&1 \
  && ok "nginx, httpd, mariadb, valkey, certbot, clamav, bind, nftables, archives, ACL, bubblewrap, utilities" || die "base package installation"
command -v unar >/dev/null 2>&1 || dnf install -y unar >/dev/null 2>&1 || warn "unar could not be installed; RAR support will use bsdtar when available"

# ============ 3) PHP (8 versions + base + wp-cli) ============
step "3) PHP versions (8 Remi + base) + wp-cli"
BASE_PKGS="php php-fpm php-cli php-mysqlnd php-mbstring php-json php-pecl-redis6"
dnf install -y $BASE_PKGS >/dev/null 2>&1 && ok "base php + php-redis"
for v in $PHP_VERS; do
  pkgs=""; for e in $PHP_EXT; do pkgs="$pkgs php$v-php-$e"; done
  dnf install -y $pkgs php$v-php-pecl-redis6 >/dev/null 2>&1 && ok "php$v (+redis)" || warn "some php$v packages were skipped"
done
if [ ! -x /usr/local/bin/wp ]; then
  curl -fsSL -o /usr/local/bin/wp https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar 2>/dev/null \
    && chmod +x /usr/local/bin/wp && ok "wp-cli" || warn "wp-cli could not be downloaded (required for WordPress features)"
else ok "wp-cli (existing)"; fi

# ============ 4) MARIADB ============
step "4) MariaDB"
systemctl enable --now mariadb >/dev/null 2>&1; sleep 2
systemctl is-active --quiet mariadb || die "MariaDB did not start"
DBPASS=$(openssl rand -hex 16)
mysql -u root <<SQL
CREATE DATABASE IF NOT EXISTS panel CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'panel'@'127.0.0.1' IDENTIFIED BY '$DBPASS';
ALTER USER 'panel'@'127.0.0.1' IDENTIFIED BY '$DBPASS';
GRANT ALL PRIVILEGES ON panel.* TO 'panel'@'127.0.0.1';
FLUSH PRIVILEGES;
SQL
ok "panel DB + user (panel@127.0.0.1)"

# ============ 5) DIRECTORIES + ENV ============
step "5) Directories + environment"
mkdir -p /opt/servika/bin /opt/servika/frontend-dist /opt/servika/src/migrations \
         /opt/servika/pma-signon /etc/servika /etc/ssl/servika
JWT=$(openssl rand -hex 32); RADMIN=$(openssl rand -hex 24)
cat > /etc/servika/env <<ENV
SERVIKA_LISTEN=127.0.0.1:8080
SERVIKA_ENV=production
SERVIKA_DB_DSN=panel:${DBPASS}@tcp(127.0.0.1:3306)/panel?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci
SERVIKA_JWT_SECRET=${JWT}
SERVIKA_JWT_LIFETIME_SEC=43200
SERVIKA_REDIS_ADMIN_PASS=${RADMIN}
ENV
chmod 600 /etc/servika/env
ok "/etc/servika/env (JWT + DB DSN + Redis administrator generated)"

# ============ 6) ARTIFACT DEPLOYMENT ============
step "6) Panel binary + frontend + migrations"
install -m 0755 "$A/servika-server" /opt/servika/bin/servika-server
[ -f "$A/servika-seed-admin" ] && install -m 0755 "$A/servika-seed-admin" /opt/servika/bin/servika-seed-admin
tar xzf "$A/frontend-dist.tar.gz" -C /opt/servika/frontend-dist && ok "frontend-dist"
tar xzf "$A/migrations.tar.gz" -C /opt/servika/src/migrations && ok "migrations ($(ls /opt/servika/src/migrations/*.sql 2>/dev/null | wc -l) SQL)"
# Operations tools and phpMyAdmin signon
for t in "$A"/ops/*; do
  bn=$(basename "$t"); nm="${bn%.sh}"
  install -m 0755 "$t" "/usr/local/bin/$nm" 2>/dev/null
done
cp "$A/ops/"* /opt/servika/src/scripts/ 2>/dev/null
ok "operations tools (/usr/local/bin: update, optimize, redis-setup, ftp-setup, backup-all, repair, jail, wp-redis)"

# ============ 7) PANEL SSL (self-signed) ============
step "7) Panel SSL (:8443 self-signed)"
if [ ! -f /etc/ssl/servika/panel.crt ]; then
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
    -keyout /etc/ssl/servika/panel.key -out /etc/ssl/servika/panel.crt \
    -subj "/CN=servika" >/dev/null 2>&1
fi
chmod 600 /etc/ssl/servika/panel.key
ok "panel.crt / panel.key"

# ============ 8) NGINX ============
step "8) nginx (panel vhost + phpMyAdmin + perf)"
# Apply the HTTP-level client_max_body_size setting idempotently.
# Do not add server_names_hash_bucket_size because servika-optimize already defines it
# in 00-perf.conf, and defining it here would make nginx -t report a duplicate directive.
grep -q "client_max_body_size 10240m" /etc/nginx/nginx.conf || \
  sed -i '/^http {/a\    client_max_body_size 10240m;' /etc/nginx/nginx.conf
cp "$A/nginx/_panel.conf"    /etc/nginx/conf.d/_panel.conf
cp "$A/nginx/_default80.conf" /etc/nginx/conf.d/_default80.conf
cp "$A/nginx/php-fpm.conf"    /etc/nginx/conf.d/php-fpm.conf 2>/dev/null
nginx -t >/dev/null 2>&1 && ok "nginx -t OK" || { nginx -t; die "nginx configuration error"; }

# ============ 9) phpMyAdmin ============
step "9) phpMyAdmin"
mkdir -p /opt/phpmyadmin   # Create this first so extraction with strip-components succeeds.
if [ ! -f /opt/phpmyadmin/index.php ]; then
  TMP=$(mktemp -d)
  if curl -fsSL -o "$TMP/pma.tar.gz" https://www.phpmyadmin.net/downloads/phpMyAdmin-latest-all-languages.tar.gz \
     && tar xzf "$TMP/pma.tar.gz" -C /opt/phpmyadmin --strip-components=1; then
    ok "phpMyAdmin downloaded + extracted"
  else warn "phpMyAdmin could not be downloaded (network issue?), run servika-repair manually later"; fi
  rm -rf "$TMP"
fi
if [ -f "$A/phpmyadmin/config.inc.php" ]; then
  BLOWFISH=$(openssl rand -hex 16)           # Generate a fresh production secret.
  PMACTRL=$(openssl rand -hex 16)            # Generate a fresh control-user password.
  sed -e "s/BLOWFISH_SECRET_PLACEHOLDER/$BLOWFISH/g" -e "s/PMA_CONTROL_PASS_PLACEHOLDER/$PMACTRL/g" \
    "$A/phpmyadmin/config.inc.php" > /opt/phpmyadmin/config.inc.php
  # Create the control user, phpMyAdmin database, and pmadb tables for advanced features.
  mysql -u root <<SQL 2>/dev/null
CREATE DATABASE IF NOT EXISTS phpmyadmin;
CREATE USER IF NOT EXISTS 'pma'@'127.0.0.1' IDENTIFIED BY '$PMACTRL';
CREATE USER IF NOT EXISTS 'pma'@'localhost' IDENTIFIED BY '$PMACTRL';
ALTER USER 'pma'@'127.0.0.1' IDENTIFIED BY '$PMACTRL';
ALTER USER 'pma'@'localhost' IDENTIFIED BY '$PMACTRL';
GRANT ALL PRIVILEGES ON phpmyadmin.* TO 'pma'@'127.0.0.1', 'pma'@'localhost';
FLUSH PRIVILEGES;
SQL
  [ -f /opt/phpmyadmin/sql/create_tables.sql ] && mysql -u root phpmyadmin < /opt/phpmyadmin/sql/create_tables.sql 2>/dev/null
fi
[ -f "$A/phpmyadmin/pma-signon.php" ] && cp "$A/phpmyadmin/pma-signon.php" /opt/servika/pma-signon/ 2>/dev/null
openssl rand -hex 32 > /etc/servika/pma-internal.token
chown root:apache /etc/servika/pma-internal.token
chmod 0640 /etc/servika/pma-internal.token
cp "$A/php-fpm/phpmyadmin.conf" /etc/php-fpm.d/phpmyadmin.conf
mkdir -p /var/lib/phpmyadmin/{tmp,sessions}
chown -R nginx:nginx /opt/phpmyadmin /var/lib/phpmyadmin 2>/dev/null
restorecon -R /opt/phpmyadmin /var/lib/phpmyadmin >/dev/null 2>&1
setsebool -P httpd_can_network_connect_db 1 >/dev/null 2>&1
ok "phpMyAdmin pool + configuration + permissions"

# ============ 10) systemd + services ============
step "10) systemd + services"
cp "$A/systemd/servika.service" /etc/systemd/system/servika.service
systemctl daemon-reload
systemctl enable --now php-fpm >/dev/null 2>&1
for v in $PHP_VERS; do systemctl enable --now php$v-php-fpm >/dev/null 2>&1; done
ok "php-fpm (base + 5 versions)"

# ---- named authoritative DNS server for hosted domains ----
NC=/etc/named.conf
if [ -f "$NC" ]; then
  cp -a "$NC" "$NC.servika-bak" 2>/dev/null || true
  # Listen on every interface so external clients can query authoritative zones.
  sed -i -E 's/listen-on port 53 \{[^}]*\}/listen-on port 53 { any; }/' "$NC"
  sed -i -E 's/listen-on-v6 port 53 \{[^}]*\}/listen-on-v6 port 53 { any; }/' "$NC"
  # Disable recursion to prevent an open resolver and DNS amplification abuse.
  sed -i -E 's/recursion yes/recursion no/' "$NC"
  # Add the panel-managed zone include idempotently; WriteZone populates it.
  grep -q 'servika-zones.conf' "$NC" || \
    echo 'include "/etc/named/servika-zones.conf";' >> "$NC"
fi
# Initialize the panel-managed zone include; domain provisioning populates it.
mkdir -p /etc/named
[ -f /etc/named/servika-zones.conf ] || \
  printf '// servika, generated automatically\n' > /etc/named/servika-zones.conf
chown root:named /etc/named/servika-zones.conf 2>/dev/null || true
chmod 640 /etc/named/servika-zones.conf 2>/dev/null || true
# Zone files under /var/named require the SELinux named_zone_t context.
restorecon -R /var/named /etc/named >/dev/null 2>&1 || true
if named-checkconf >/dev/null 2>&1; then
  systemctl enable --now named >/dev/null 2>&1 && ok "named (authoritative DNS, :53 open, recursion disabled)" || warn "named could not be started"
else
  warn "named-checkconf error, DNS must be checked manually"
fi

# ---- acme.sh for Let's Encrypt SSL; the panel invokes /root/.acme.sh/acme.sh ----
# Let's Encrypt requires a valid email address; register without contact information otherwise.
AEMAIL="$ADMIN_EMAIL"; echo "$AEMAIL" | grep -qE '@[^@]+\.[^@]+$' || AEMAIL=""
if [ ! -x /root/.acme.sh/acme.sh ]; then
  if [ -n "$AEMAIL" ]; then curl -fsSL https://get.acme.sh 2>/dev/null | sh -s email="$AEMAIL" >/dev/null 2>&1 || true
  else curl -fsSL https://get.acme.sh 2>/dev/null | sh >/dev/null 2>&1 || true; fi
fi
if [ -x /root/.acme.sh/acme.sh ]; then
  /root/.acme.sh/acme.sh --set-default-ca --server letsencrypt >/dev/null 2>&1
  # Register the account now so certificate issuance does not fail later.
  if [ -n "$AEMAIL" ]; then /root/.acme.sh/acme.sh --register-account -m "$AEMAIL" --server letsencrypt >/dev/null 2>&1
  else /root/.acme.sh/acme.sh --register-account --server letsencrypt >/dev/null 2>&1; fi
  ok "acme.sh (Let's Encrypt CA + account registered + automatic renewal cron)"
else
  warn "acme.sh could not be installed, install it manually for Let's Encrypt SSL: curl https://get.acme.sh | sh"
fi

# ---- httpd backend for web_backend=apache, with nginx as the reverse proxy ----
# Apache listens on 127.0.0.1:10080 because nginx owns port 80.
if [ -f /etc/httpd/conf/httpd.conf ]; then
  if grep -qE "^Listen 80$" /etc/httpd/conf/httpd.conf; then
    sed -i "s/^Listen 80$/Listen 127.0.0.1:10080/" /etc/httpd/conf/httpd.conf
  elif ! grep -qE "^Listen 127.0.0.1:10080" /etc/httpd/conf/httpd.conf; then
    echo "Listen 127.0.0.1:10080" >> /etc/httpd/conf/httpd.conf
  fi
  semanage port -l 2>/dev/null | grep -qE "http_port_t.*\b10080\b" || \
    semanage port -a -t http_port_t -p tcp 10080 2>/dev/null || \
    semanage port -m -t http_port_t -p tcp 10080 2>/dev/null
  if apachectl configtest >/dev/null 2>&1; then
    systemctl enable --now httpd >/dev/null 2>&1 && ok "httpd (Apache backend :10080, mod_proxy_fcgi)" || warn "httpd could not be started"
  else warn "httpd configtest error, check the Apache backend manually"; fi
fi

# ---- Composer for per-domain PHP dependency management ----
if [ ! -x /usr/local/bin/composer ]; then
  curl -sS https://getcomposer.org/installer 2>/dev/null | php -- --install-dir=/usr/local/bin --filename=composer >/dev/null 2>&1
fi
[ -x /usr/local/bin/composer ] && ok "composer ($(/usr/local/bin/composer --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1))" || warn "composer could not be installed"

# ---- Daily backup cron using servika-backup-all at 03:00 UTC ----
cat > /etc/cron.d/servika-backup <<'CRON'
# Servika daily scheduled backup at 03:00 UTC
SHELL=/bin/bash
PATH=/usr/local/bin:/usr/bin:/bin
0 3 * * * root /usr/local/bin/servika-backup-all
CRON
ok "daily backup cron (03:00 UTC)"

# SELinux
setsebool -P httpd_can_network_connect 1 >/dev/null 2>&1 && ok "SELinux httpd_can_network_connect"
if command -v getenforce >/dev/null 2>&1 \
  && [ "$(getenforce)" != "Disabled" ] \
  && command -v semanage >/dev/null 2>&1; then
  fcontext_list=$(semanage fcontext -l 2>/dev/null || true)
  case "$fcontext_list" in
    *'/run/php-fpm-['*) ;;
    *) semanage fcontext -a -t httpd_var_run_t '/run/php-fpm-[^/]+(/.*)?' >/dev/null 2>&1 || true ;;
  esac
  ok "SELinux fcontext for per-tenant PHP-FPM sockets"
fi
restorecon -R /opt/servika/bin /opt/servika/frontend-dist >/dev/null 2>&1

# ============ 11) Valkey + optimization ============
step "11) Valkey (Redis) + performance tuning"
command -v servika-redis-setup >/dev/null 2>&1 && servika-redis-setup >/dev/null 2>&1 && ok "servika-redis-setup" || warn "redis-setup skipped"
command -v servika-optimize >/dev/null 2>&1 && servika-optimize >/dev/null 2>&1 && ok "servika-optimize" || warn "optimization skipped"

# ============ 12) START PANEL; MIGRATIONS RUN AT STARTUP ============
step "12) Starting panel"
systemctl enable --now servika >/dev/null 2>&1; sleep 3
systemctl enable --now nginx >/dev/null 2>&1; systemctl restart nginx >/dev/null 2>&1
if systemctl is-active --quiet servika; then ok "servika ACTIVE"; else journalctl -u servika --no-pager -n 20; die "panel did not start"; fi

# ---- Run the Pure-FTPd setup after migrations create the ftp_accounts table ----
# Running this in step 11 would make GRANT SELECT fail because the table does not exist yet.
sleep 2
command -v servika-ftp-setup >/dev/null 2>&1 && servika-ftp-setup >/dev/null 2>&1 && ok "servika-ftp-setup (Pure-FTPd, MySQL backend)" || warn "ftp-setup skipped"

# ============ 13) ADMINISTRATOR ACCESS ============
# Panel administrator login authenticates the server's root account through PAM and shadow.
# There is no separate panel password; use root and the server's root password.
step "13) Administrator access (root + PAM)"
DSN="panel:${DBPASS}@tcp(127.0.0.1:3306)/panel?parseTime=true"
if [ -x /opt/servika/bin/servika-seed-admin ]; then
  # Seed the users record for ownership and audit; login still uses root through PAM.
  /opt/servika/bin/servika-seed-admin -dsn "$DSN" -username root \
    -password "$(openssl rand -hex 16)" -email "$ADMIN_EMAIL" >/dev/null 2>&1 \
    && ok "administrator record ready" || warn "seed skipped (not critical)"
fi
# Clear seed defaults so the root profile starts empty and can be completed in the profile page.
mysql panel -e "UPDATE users SET email='', full_name='' WHERE username='root' AND email='admin@local';" >/dev/null 2>&1 || true
ok "Login: user 'root' + this server's root password"

# ============ 14) PERMISSION REPAIR ============
step "14) Permission/SELinux repair"
command -v servika-repair >/dev/null 2>&1 && servika-repair --quiet >/dev/null 2>&1 && ok "servika-repair" || warn "repair skipped"

# ============ 15) VERIFICATION ============
step "15) Verification"
IP=$(hostname -I 2>/dev/null | awk '{print $1}')
CODE=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/ 2>/dev/null)
API=$(curl -sk -o /dev/null -w '%{http_code}' https://127.0.0.1:8443/api/v1/domains 2>/dev/null)
echo -e "  services: $(systemctl is-active mariadb nginx valkey php-fpm named pure-ftpd servika | tr '\n' ' ')"
echo -e "  panel :8443 → HTTP $CODE   ·   API (auth) → HTTP $API   ·   DNS :53 → $(systemctl is-active named)   ·   FTP :21 → $(systemctl is-active pure-ftpd)"
echo -e "  utilities: SSL/acme.sh $([ -x /root/.acme.sh/acme.sh ] && echo ✓ || echo ✗)   ·   firewall/nft $(command -v nft >/dev/null && echo ✓ || echo ✗)   ·   unzip/zip $(command -v unzip >/dev/null && command -v zip >/dev/null && echo ✓ || echo ✗)   ·   composer $(command -v composer >/dev/null && echo ✓ || echo ✗)   ·   apache/httpd $(systemctl is-active httpd)"
echo -e "  isolation: plan-driven cgroup limits + per-tenant PHP-FPM ready   ·   bubblewrap $(command -v bwrap >/dev/null && echo ✓ || echo ✗)"
echo
echo -e "${c_g}═══════════════════════════════════════════════${c_0}"
echo -e "${c_g} ✓ Servika installation complete${c_0}"
echo -e "   Panel:  ${c_b}https://${IP:-SERVER_IP}:8443${c_0}"
echo -e "   User: ${c_b}root${c_0}   Password: ${c_b}this server's root password${c_0}"
echo -e "   (panel administrator login authenticates the server's root account through PAM)"
echo -e "${c_g}═══════════════════════════════════════════════${c_0}"
