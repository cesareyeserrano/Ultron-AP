# User Personas

**Last Updated:** 2026-02-09

---

## Admin (Primary) {#admin}

**Name:** Admin
**Role:** Raspberry Pi Owner/Operator
**Type:** Primary

### Profile
Propietario de la Raspberry Pi "Ultron" que ejecuta multiples servicios (Docker containers y Systemd services). Necesita visibilidad centralizada y control remoto sobre el estado de salud de su sistema sin conectarse por SSH.

### Goals
- Ver el estado de todos los servicios en un solo lugar
- Ser notificado inmediatamente cuando algo falla
- Reiniciar servicios caidos sin SSH
- Mantener el sistema funcionando de forma estable

### Pain Points
- Tener que SSH para verificar estado del sistema
- No enterarse de fallos hasta que afectan a otros servicios
- Recordar comandos Docker y Systemctl para cada servicio
- No tener un historico visual de metricas

### Tech Proficiency
Alta - familiarizado con Linux, Docker, Systemd, networking. Prefiere herramientas eficientes y profesionales sobre interfaces simplificadas.

### Access Context
- Accede via red local (LAN) o VPN (Tailscale/WireGuard)
- Usa desktop, tablet, o movil indistintamente
- Unico usuario del sistema

---
