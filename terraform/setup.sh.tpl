#!/bin/bash
set -euo pipefail
exec > /var/log/mailpit-setup.log 2>&1

echo "=== Mailpit VM setup started ==="

# ---------------------------------------------------------------------------
# Packages
# ---------------------------------------------------------------------------
apt-get update
apt-get install -y certbot python3-certbot-dns-cloudflare

# ---------------------------------------------------------------------------
# Mailpit user & directories
# ---------------------------------------------------------------------------
useradd --system --no-create-home --shell /usr/sbin/nologin mailpit || true
mkdir -p /etc/mailpit/tls

# ---------------------------------------------------------------------------
# Download mailpit binary
# ---------------------------------------------------------------------------
%{ if mailpit_download_url != "" ~}
DOWNLOAD_URL="${mailpit_download_url}"
%{ else ~}
DOWNLOAD_URL="https://github.com/bbathow-private/mailpit/releases/download/v${mailpit_version}/mailpit-linux-amd64.tar.gz"
%{ endif ~}

echo "Downloading mailpit from $DOWNLOAD_URL"
curl -sL "$DOWNLOAD_URL" | tar xz -C /usr/local/bin/ mailpit
chmod 755 /usr/local/bin/mailpit
setcap 'cap_net_bind_service=+ep' /usr/local/bin/mailpit

# ---------------------------------------------------------------------------
# Bounce rules
# ---------------------------------------------------------------------------
cat > /etc/mailpit/bounce-rules.yaml << 'BOUNCE_EOF'
${bounce_rules}
BOUNCE_EOF

# ---------------------------------------------------------------------------
# Cloudflare credentials for certbot DNS challenge
# ---------------------------------------------------------------------------
mkdir -p /etc/letsencrypt
cat > /etc/letsencrypt/cloudflare.ini << CF_EOF
dns_cloudflare_api_token = ${cloudflare_api_token}
CF_EOF
chmod 600 /etc/letsencrypt/cloudflare.ini

# ---------------------------------------------------------------------------
# Obtain TLS certificate
# ---------------------------------------------------------------------------
echo "Requesting TLS certificate for ${domain}..."
for i in 1 2 3 4 5; do
  certbot certonly \
    --dns-cloudflare \
    --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
    --dns-cloudflare-propagation-seconds 30 \
    -d "${domain}" \
    --non-interactive \
    --agree-tos \
    -m "${letsencrypt_email}" && break
  echo "certbot attempt $i failed, retrying in 30s..."
  sleep 30
done

# Copy certs to mailpit-readable location
cp /etc/letsencrypt/live/${domain}/fullchain.pem /etc/mailpit/tls/
cp /etc/letsencrypt/live/${domain}/privkey.pem /etc/mailpit/tls/
chown -R mailpit:mailpit /etc/mailpit/tls
chmod 600 /etc/mailpit/tls/privkey.pem

# ---------------------------------------------------------------------------
# Cert renewal hook — copy new certs and restart mailpit
# ---------------------------------------------------------------------------
mkdir -p /etc/letsencrypt/renewal-hooks/deploy
cat > /etc/letsencrypt/renewal-hooks/deploy/mailpit.sh << 'HOOK_EOF'
#!/bin/bash
cp /etc/letsencrypt/live/${domain}/fullchain.pem /etc/mailpit/tls/
cp /etc/letsencrypt/live/${domain}/privkey.pem /etc/mailpit/tls/
chown mailpit:mailpit /etc/mailpit/tls/*
chmod 600 /etc/mailpit/tls/privkey.pem
systemctl restart mailpit
HOOK_EOF
chmod +x /etc/letsencrypt/renewal-hooks/deploy/mailpit.sh

# ---------------------------------------------------------------------------
# Optional: basic auth for web UI
# ---------------------------------------------------------------------------
%{ if ui_auth_enabled ~}
apt-get install -y apache2-utils
htpasswd -cb /etc/mailpit/ui-auth "${ui_auth_user}" "${ui_auth_password}"
chown mailpit:mailpit /etc/mailpit/ui-auth
chmod 600 /etc/mailpit/ui-auth
%{ endif ~}

# ---------------------------------------------------------------------------
# Systemd service
# ---------------------------------------------------------------------------
cat > /etc/systemd/system/mailpit.service << 'SVC_EOF'
[Unit]
Description=Mailpit - Email testing tool
After=network.target

[Service]
Type=simple
User=mailpit
Group=mailpit
ExecStart=/usr/local/bin/mailpit \
  --smtp 0.0.0.0:25 \
  --listen 0.0.0.0:443 \
  --ui-tls-cert /etc/mailpit/tls/fullchain.pem \
  --ui-tls-key /etc/mailpit/tls/privkey.pem \
  --smtp-tls-cert /etc/mailpit/tls/fullchain.pem \
  --smtp-tls-key /etc/mailpit/tls/privkey.pem \
  --enable-bounce-rules \
  --bounce-rules-file /etc/mailpit/bounce-rules.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=false
ProtectSystem=strict
ReadWritePaths=/tmp
ReadOnlyPaths=/etc/mailpit

[Install]
WantedBy=multi-user.target
SVC_EOF

%{ if ui_auth_enabled ~}
# Append --ui-auth-file to ExecStart
sed -i 's|--bounce-rules-file /etc/mailpit/bounce-rules.yaml|--bounce-rules-file /etc/mailpit/bounce-rules.yaml \\\n  --ui-auth-file /etc/mailpit/ui-auth|' /etc/systemd/system/mailpit.service
%{ endif ~}

systemctl daemon-reload
systemctl enable mailpit
systemctl start mailpit

echo "=== Mailpit VM setup complete ==="
echo "Web UI: https://${domain}"
echo "SMTP:   ${domain}:25"
