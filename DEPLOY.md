# Deployment & HTTPS Guide

This guide explains how to deploy Ultron-AP securely using HTTPS, which is recommended if you plan to access the panel over the internet.

## Option 1: Tailscale (Recommended for Private Access)

Tailscale is the easiest way to get HTTPS without managing certificates or opening firewall ports.

1.  **Install Tailscale** on your Raspberry Pi: `curl -fsSL https://tailscale.com/install.sh | sh`
2.  **Enable HTTPS** in your Tailscale admin console (under Settings > DNS).
3.  **Use the MagicDNS name**: Access your Pi at `https://your-pi-name.tailnet-name.ts.net`.
4.  Ultron-AP's built-in Tailscale integration will automatically show peer status in the dashboard.

## Option 2: Caddy Reverse Proxy (Public Access)

If you want to access Ultron-AP via a public domain name, Caddy is the best lightweight choice.

### 1. Install Caddy
```bash
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install caddy
```

### 2. Configure Caddyfile
Edit `/etc/caddy/Caddyfile`:
```caddy
your-domain.com {
    reverse_proxy localhost:8080
}
```

### 3. Restart Caddy
```bash
sudo systemctl restart caddy
```

## Security Best Practices

- **Firewall**: If using a reverse proxy, ensure port `8080` is only accessible from `localhost`.
- **Brute-Force**: Ultron-AP has built-in brute-force protection (3 attempts / 15m), but for public exposure, consider a WAF like Cloudflare.
- **VPN**: For maximum security, only expose Ultron-AP via a VPN (Tailscale/Wireguard) and never open ports on your router.

## Host Hardening (Recommended)

### 1) Install privileged helper (root boundary)
```bash
sudo install -m 0755 /opt/ultron-ap/ultron-helper /opt/ultron-ap/ultron-helper
sudo install -m 0644 deploy/ultron-helper.service /etc/systemd/system/ultron-helper.service
sudo systemctl daemon-reload
sudo systemctl enable --now ultron-helper
```

### 2) Install hardened web unit
```bash
sudo install -m 0644 deploy/ultron-ap.service /etc/systemd/system/ultron-ap.service
sudo systemctl daemon-reload
sudo systemctl restart ultron-ap
```

### 3) Remove legacy sudoers policy (no longer required)
```bash
sudo rm -f /etc/sudoers.d/ultron-ap /etc/sudoers.d/ultron-logs
```

### 4) Validate effective hardening
```bash
systemctl show ultron-ap -p NoNewPrivileges -p ProtectSystem -p PrivateTmp -p ProtectKernelTunables -p RestrictSUIDSGID
systemctl is-active ultron-helper ultron-ap
ls -l /run/ultron-helper.sock
```

`NoNewPrivileges=true` is now enforced on `ultron-ap`; privileged operations execute only via `ultron-helper` over local Unix socket.
