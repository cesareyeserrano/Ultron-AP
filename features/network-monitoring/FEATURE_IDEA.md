## Feature
Network Monitoring — observabilidad continua de la red doméstica (LAN + enlace a Internet) desde Ultron en el Raspberry Pi.

## Problem / Why
Hoy no hay visibilidad sobre la salud de la red de la casa. Cuando algo va lento (videollamadas, streaming, juegos, IoT) no se sabe si el problema es el ISP, el router, el WiFi o un dispositivo concreto. Ultron ya está corriendo 24/7 en la red, así que es la pieza natural para medirla y guardar histórico.

## Target Users
Usuario único de Ultron (admin/dueño de la red doméstica). No hay multi-tenant.

## New Behavior
El sistema debe medir, almacenar y exponer métricas de red de forma continua, balanceando exactitud contra coste de CPU/RAM/disco/ancho de banda en el Pi y en el propio enlace.

### Métricas candidatas (a priorizar en discovery — no todas entran en v1)

**Latencia y calidad del path**
- RTT (latencia ICMP/UDP/TCP) a múltiples targets (gateway, DNS público, anycast tipo 1.1.1.1 / 8.8.8.8, un host del ISP).
- **Jitter** (variación del RTT entre muestras consecutivas).
- Pérdida de paquetes (% sobre ventana móvil).
- MOS / R-factor estimado para voz (derivado de latencia + jitter + pérdida).
- Hops y ruta (traceroute periódico, detección de cambios de ruta / asimetrías).
- Path MTU / fragmentación.
- TCP retransmits (si se puede leer de `/proc/net/netstat`).
- Bufferbloat (latencia bajo carga vs. en reposo — estilo `flent` / `dslreports`).

**Capacidad / ancho de banda**
- Throughput de bajada y subida (speedtest periódico — Ookla, iperf3, librespeed).
- Bytes/s en la interfaz del Pi (eth/wlan) — bandwidth real consumido.
- Top talkers en la LAN (si el Pi puede inspeccionar tráfico que ve, sin DPI).
- Saturación del enlace (uso vs. capacidad nominal).

**Disponibilidad / continuidad**
- Uptime del enlace WAN (% tiempo con Internet alcanzable).
- Detección de caídas y duración de cada incidente.
- IP pública actual y cambios de IP (eventos).
- Estado del gateway (alcanzable sí/no).

**DNS**
- Tiempo de resolución a un set de dominios (dominio local del ISP, populares, propio).
- Tasa de fallo / NXDOMAIN inesperados.
- Diferencia entre DNS del ISP vs. público (1.1.1.1, 9.9.9.9).

**WiFi** (si el Pi tiene radio o podemos consultar al router vía SNMP/SSH/API)
- RSSI, SNR, calidad de señal.
- Canal, banda (2.4/5/6 GHz), ancho de canal.
- Tasa de PHY negociada, retries, errores CRC.
- Clientes conectados, roaming events.

**LAN / dispositivos**
- Inventario de hosts activos (ARP/mDNS/NDP), aparición y desaparición.
- Latencia intra-LAN al gateway y a hosts críticos.
- Conteo de dispositivos activos a lo largo del día.

**Sistema (coste de medir)**
- CPU/RAM/IO consumidos por el propio monitor.
- Ancho de banda consumido por las sondas (especialmente speedtests).
- Espacio en disco del histórico.
- Temperatura del Pi (las sondas suben CPU → calor).

### Almacenamiento y consulta
- Histórico consultable: tiempo real, últimas 24h, 7d, 30d con downsampling.
- API para que otros componentes de Ultron / dashboards consuman métricas.
- Alertas básicas cuando una métrica cruza umbral sostenido (latencia, pérdida, bufferbloat, caída WAN, cambio de IP pública).

## Success Criteria
- **Given** Ultron corriendo en el Pi, **When** consulto el dashboard/API, **Then** veo estado actual e histórico de las métricas clave (al menos: latencia, jitter, pérdida, throughput, uptime WAN) en los últimos 7 días.
- **Given** una caída o degradación sostenida del enlace, **When** ocurre, **Then** queda registrada con timestamp y, si se configuró, dispara alerta.
- **Given** el monitor en estado estable, **When** mido el coste, **Then** el overhead se mantiene dentro del presupuesto explícito definido en discovery (CPU promedio, RAM, IO, ancho de banda de sondas, espacio histórico).
- El feature no degrada funciones existentes de Ultron ni el rendimiento percibido de la red durante uso normal (sondas activas como speedtest deben ser planificables / evitables en horas pico).

## Out of Scope
- Pentesting, detección de intrusos, inventario de seguridad.
- Deep Packet Inspection / packet capture continuo.
- QoS / traffic shaping / control parental.
- Multi-site (solo una red doméstica).
- Reemplazar herramientas tipo Grafana/Prometheus — se decide en arquitectura si se reusan o se hace algo nativo a Ultron.

## Notes for Discovery
El usuario pidió explícitamente un **discovery detallado** antes de comprometerse a un alcance, evaluando coste vs. valor de cada métrica. Phase 1 debe entregar:
1. Lista priorizada de métricas (must / should / could / won't para v1).
2. Presupuesto de recursos (CPU, RAM, disco, BW de sondas) y método para medir si se respeta.
3. Decisión sobre sondas activas (speedtest, iperf) vs. pasivas (lectura de contadores) — frecuencia, ventana, opt-in.
4. Estrategia de retención e histórico.
