# Deploying Mailpit

Two deployment options depending on how external mail reaches mailpit:

| Option | SMTP flow | Bounce rule responses |
|---|---|---|
| **A. Dedicated VM** | Sender MTA → mailpit directly on port 25 | Immediate — sender MTA sees mailpit's SMTP response |
| **B. Behind Postfix** | Sender MTA → Postfix → mailpit on alt port | Delayed — Postfix queues, retries on 4xx, generates DSN |

**Option A is recommended** when testing bounce rules, because the sending MTA receives mailpit's SMTP error codes (451, 550, etc.) in real time.

Replace these placeholders throughout:

| Placeholder | Example | Description |
|---|---|---|
| `VM_USER` | `root` | SSH user on the VM |
| `VM_HOST` | `203.0.113.10` | VM IP or hostname |
| `MAILPIT_DOMAIN` | `mailpit.example.com` | Domain for mailpit |

---

## Build the binary (local machine)

```bash
cd /path/to/mailpit
npm install && npm run package
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-s -w -X github.com/axllent/mailpit/config.Version=$(git describe --tags --always)" \
  -o mailpit-linux-amd64
```

## Create bounce rules config (local machine)

```bash
cat > bounce-rules.yaml << 'EOF'
rules:
  - field: subject
    pattern: "BOUNCE-HARD"
    action: hard_bounce
    priority: 1

  - field: subject
    pattern: "BOUNCE-SOFT"
    action: soft_bounce
    priority: 2

  - field: to
    pattern: "hard-bounce@"
    action: hard_bounce
    priority: 3

  - field: header
    header_name: X-Test-Bounce
    pattern: "^reject$"
    action: reject
    priority: 4

  - field: header
    header_name: X-Test-Bounce
    pattern: "^quota$"
    action: quota_full
    priority: 5
EOF
```

> **Note:** If the VM runs rspamd or SpamAssassin, avoid patterns that trigger
> spam filters (e.g. "BOUNCE" in the subject). Use custom headers like
> `X-Test-Bounce` instead, or whitelist mailpit's domain in the spam filter.

## Copy files to the VM

```bash
scp mailpit-linux-amd64 VM_USER@VM_HOST:/tmp/mailpit
scp bounce-rules.yaml VM_USER@VM_HOST:/tmp/bounce-rules.yaml
```

## Install on the VM

```bash
ssh VM_USER@VM_HOST
```

```bash
sudo install -m 755 /tmp/mailpit /usr/local/bin/mailpit
sudo mkdir -p /etc/mailpit
sudo install -m 644 /tmp/bounce-rules.yaml /etc/mailpit/bounce-rules.yaml
sudo useradd --system --no-create-home --shell /usr/sbin/nologin mailpit
```

---

# Option A: Dedicated VM (standalone, direct SMTP)

Mailpit listens on port 25 (SMTP) and port 443 (web UI) with TLS.
No Postfix, no reverse proxy.

For automated provisioning on Azure, see the `terraform/` directory.

## 1. Obtain a TLS certificate

```bash
# Install certbot with Cloudflare DNS plugin
sudo apt-get install -y certbot python3-certbot-dns-cloudflare

# Write Cloudflare API credentials
sudo mkdir -p /etc/letsencrypt
cat << EOF | sudo tee /etc/letsencrypt/cloudflare.ini > /dev/null
dns_cloudflare_api_token = YOUR_CLOUDFLARE_API_TOKEN
EOF
sudo chmod 600 /etc/letsencrypt/cloudflare.ini

# Request certificate
sudo certbot certonly \
  --dns-cloudflare \
  --dns-cloudflare-credentials /etc/letsencrypt/cloudflare.ini \
  -d MAILPIT_DOMAIN \
  --non-interactive --agree-tos -m you@example.com

# Copy certs to mailpit-readable location
sudo mkdir -p /etc/mailpit/tls
sudo cp /etc/letsencrypt/live/MAILPIT_DOMAIN/fullchain.pem /etc/mailpit/tls/
sudo cp /etc/letsencrypt/live/MAILPIT_DOMAIN/privkey.pem /etc/mailpit/tls/
sudo chown -R mailpit:mailpit /etc/mailpit/tls
sudo chmod 600 /etc/mailpit/tls/privkey.pem
```

## 2. Allow binding to privileged ports

```bash
sudo setcap 'cap_net_bind_service=+ep' /usr/local/bin/mailpit
```

## 3. Create systemd service

```bash
sudo tee /etc/systemd/system/mailpit.service > /dev/null << 'EOF'
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
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/tmp
ReadOnlyPaths=/etc/mailpit

[Install]
WantedBy=multi-user.target
EOF
```

## 4. Create certbot renewal hook

```bash
sudo mkdir -p /etc/letsencrypt/renewal-hooks/deploy
sudo tee /etc/letsencrypt/renewal-hooks/deploy/mailpit.sh > /dev/null << 'EOF'
#!/bin/bash
cp /etc/letsencrypt/live/MAILPIT_DOMAIN/fullchain.pem /etc/mailpit/tls/
cp /etc/letsencrypt/live/MAILPIT_DOMAIN/privkey.pem /etc/mailpit/tls/
chown mailpit:mailpit /etc/mailpit/tls/*
chmod 600 /etc/mailpit/tls/privkey.pem
systemctl restart mailpit
EOF
sudo chmod +x /etc/letsencrypt/renewal-hooks/deploy/mailpit.sh
```

## 5. Start the service

```bash
sudo systemctl daemon-reload
sudo systemctl enable mailpit
sudo systemctl start mailpit
```

## 6. DNS records

Create an **A record** and **MX record** for `MAILPIT_DOMAIN`:

| Type | Name | Content | TTL |
|---|---|---|---|
| A | `MAILPIT_DOMAIN` | VM's public IP | 300 |
| MX | `MAILPIT_DOMAIN` | `MAILPIT_DOMAIN` | 300 |

If using Cloudflare, set **DNS only** (no proxy) — Cloudflare's proxy does not support SMTP.

---

# Option B: Behind Postfix (ISPConfig shared VM)

Mailpit listens on localhost only. Postfix relays mail to it.
ISPConfig handles the web UI via reverse proxy with Let's Encrypt.

> **Important:** Postfix is store-and-forward. The sending MTA gets `250 OK`
> from Postfix immediately. Mailpit's bounce responses (451, 550, etc.) are
> only seen by Postfix, which then generates a DSN back to the sender.
> For 4xx responses (soft_bounce, quota_full), Postfix will retry for up to
> 5 days before giving up.

## 1. Create systemd service

```bash
sudo tee /etc/systemd/system/mailpit.service > /dev/null << 'EOF'
[Unit]
Description=Mailpit - Email testing tool
After=network.target

[Service]
Type=simple
User=mailpit
Group=mailpit
ExecStart=/usr/local/bin/mailpit \
  --smtp 127.0.0.1:1025 \
  --listen 127.0.0.1:8025 \
  --enable-bounce-rules \
  --bounce-rules-file /etc/mailpit/bounce-rules.yaml
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/tmp
ReadOnlyPaths=/etc/mailpit

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable mailpit
sudo systemctl start mailpit
```

## 2. Configure Postfix relay

In ISPConfig, add a relay domain, relay recipient, and transport for `MAILPIT_DOMAIN`:

| ISPConfig setting | Value |
|---|---|
| Relay domain | `MAILPIT_DOMAIN` |
| Relay recipient | `@MAILPIT_DOMAIN` |
| Transport | `MAILPIT_DOMAIN` → `smtp:127.0.0.1:1025` |

Then set this Postfix option (required for relay domains to be accepted from external senders):

```bash
sudo postconf -e "smtpd_relay_before_recipient_restrictions=yes"
sudo postfix reload
```

Without this, external mail to relay domains may be rejected with `554 5.7.1 Relay access denied` because `smtpd_recipient_restrictions` runs before relay permissions are checked.

## 3. Configure ISPConfig reverse proxy

### In the ISPConfig panel

1. Go to **Sites > Add new Website**
2. Set domain to `MAILPIT_DOMAIN`
3. Enable **SSL** and **Let's Encrypt**
4. Go to the **Options** tab

### Apache (add to Apache Directives)

```apache
<Location />
  ProxyPass http://127.0.0.1:8025/
  ProxyPassReverse http://127.0.0.1:8025/
  ProxyPreserveHost On
</Location>

# WebSocket support for live updates
<Location /api/events>
  ProxyPass ws://127.0.0.1:8025/api/events
  ProxyPassReverse ws://127.0.0.1:8025/api/events
</Location>
```

Enable the required modules:

```bash
sudo a2enmod proxy proxy_http proxy_wstunnel
sudo systemctl reload apache2
```

### Nginx (add to Nginx Directives)

```nginx
location / {
    proxy_pass http://127.0.0.1:8025;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}

location /api/events {
    proxy_pass http://127.0.0.1:8025/api/events;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
}
```

---

# Common steps (both options)

## (Optional) Add basic auth

```bash
sudo apt-get install -y apache2-utils
sudo htpasswd -c /etc/mailpit/ui-auth admin
sudo chown mailpit:mailpit /etc/mailpit/ui-auth
```

Add `--ui-auth-file /etc/mailpit/ui-auth` to ExecStart in the systemd unit:

```bash
sudo sed -i 's|--bounce-rules-file /etc/mailpit/bounce-rules.yaml|--bounce-rules-file /etc/mailpit/bounce-rules.yaml \\\n  --ui-auth-file /etc/mailpit/ui-auth|' /etc/systemd/system/mailpit.service
sudo systemctl daemon-reload
sudo systemctl restart mailpit
```

## Verify

```bash
# Check service
sudo systemctl status mailpit

# Send a normal test email (adjust port for your option)
echo -e "Subject: Test\n\nHello" | curl --url smtp://127.0.0.1:1025 \
  --mail-from test@example.com --mail-rcpt rcpt@example.com --upload-file -

# Trigger a bounce rule (expect 550 error)
echo -e "Subject: BOUNCE-HARD\n\nHello" | curl --url smtp://127.0.0.1:1025 \
  --mail-from test@example.com --mail-rcpt rcpt@example.com --upload-file -

# Check bounce rules via API
curl https://MAILPIT_DOMAIN/api/v1/bounce-rules

# View logs
sudo journalctl -u mailpit -f
```

## Update the binary

From your local machine:

```bash
scp mailpit-linux-amd64 VM_USER@VM_HOST:/tmp/mailpit
ssh VM_USER@VM_HOST "sudo install -m 755 /tmp/mailpit /usr/local/bin/mailpit && sudo systemctl restart mailpit"
```

## Update bounce rules at runtime (no restart)

```bash
curl -X PUT https://MAILPIT_DOMAIN/api/v1/bounce-rules \
  -H 'Content-Type: application/json' \
  -d '[{"field":"subject","pattern":"NEW-RULE","action":"hard_bounce"}]'
```
