# Deploying Mailpit to an ISPConfig VM

## Prerequisites

- Linux VM (amd64) with SSH key access
- ISPConfig installed (Apache or Nginx)
- A DNS A record pointing `MAILPIT_DOMAIN` to the VM's IP

Replace these placeholders throughout:

| Placeholder | Example | Description |
|---|---|---|
| `VM_USER` | `root` | SSH user on the VM |
| `VM_HOST` | `203.0.113.10` | VM IP or hostname |
| `MAILPIT_DOMAIN` | `mailpit.example.com` | Domain for the web UI |

---

## 1. Build the binary (local machine)

```bash
cd /path/to/mailpit
npm install && npm run package
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -ldflags "-s -w -X github.com/axllent/mailpit/config.Version=$(git describe --tags --always)" \
  -o mailpit-linux-amd64
```

## 2. Create bounce rules config (local machine)

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

## 3. Copy files to the VM

```bash
scp mailpit-linux-amd64 VM_USER@VM_HOST:/tmp/mailpit
scp bounce-rules.yaml VM_USER@VM_HOST:/tmp/bounce-rules.yaml
```

## 4. Install on the VM

SSH into the VM:

```bash
ssh VM_USER@VM_HOST
```

Then run:

```bash
# Install binary
sudo install -m 755 /tmp/mailpit /usr/local/bin/mailpit

# Create config directory and install bounce rules
sudo mkdir -p /etc/mailpit
sudo install -m 644 /tmp/bounce-rules.yaml /etc/mailpit/bounce-rules.yaml

# Create dedicated service user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin mailpit
```

## 5. Create systemd service

```bash
sudo cat > /etc/systemd/system/mailpit.service << 'EOF'
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
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable mailpit
sudo systemctl start mailpit
sudo systemctl status mailpit
```

Check logs:

```bash
sudo journalctl -u mailpit -f
```

## 6. Configure ISPConfig reverse proxy

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

## 7. (Optional) Add basic auth

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

---

## Verify

```bash
# Check service
sudo systemctl status mailpit

# Send a normal test email
echo -e "Subject: Test\n\nHello" | curl --url smtp://127.0.0.1:1025 \
  --mail-from test@example.com --mail-rcpt rcpt@example.com --upload-file -

# Trigger a bounce rule (expect 550 error)
echo -e "Subject: BOUNCE-HARD\n\nHello" | curl --url smtp://127.0.0.1:1025 \
  --mail-from test@example.com --mail-rcpt rcpt@example.com --upload-file -

# Check bounce rules via API
curl https://MAILPIT_DOMAIN/api/v1/bounce-rules
```

---

## Update the binary

From your local machine:

```bash
scp mailpit-linux-amd64 VM_USER@VM_HOST:/tmp/mailpit
ssh VM_USER@VM_HOST "sudo install -m 755 /tmp/mailpit /usr/local/bin/mailpit && sudo systemctl restart mailpit"
```

## Useful commands

```bash
# View logs
sudo journalctl -u mailpit -f

# Restart after config change
sudo systemctl restart mailpit

# Update bounce rules file and reload
sudo vi /etc/mailpit/bounce-rules.yaml
sudo systemctl restart mailpit

# Update bounce rules at runtime (no restart)
curl -X PUT https://MAILPIT_DOMAIN/api/v1/bounce-rules \
  -H 'Content-Type: application/json' \
  -d '[{"field":"subject","pattern":"NEW-RULE","action":"hard_bounce"}]'
```
