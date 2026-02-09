# Product Requirements Document

**Project:** Ultron-AP
**Version:** 1.0.0
**Last Updated:** 2026-02-09
**Status:** Draft

---

## 1. Project Overview

### Product Name
Ultron-AP (Ultron Admin Panel)

### Purpose
Panel de administracion ligero y profesional para la Raspberry Pi "Ultron". Proporciona un dashboard de monitoreo en tiempo real de todos los servicios (Docker y Systemd), alertas inteligentes, y controles basicos sobre los servicios — todo con un consumo minimo de recursos del sistema.

### Tech Stack
- **Backend:** Go (binario unico, ~10MB RAM)
- **Frontend:** HTMX + Tailwind CSS (interactividad sin SPA pesada)
- **Base de datos:** SQLite (configuracion, usuarios, historial de alertas)
- **Comunicacion en tiempo real:** Server-Sent Events (SSE) para metricas live
- **Templates:** Go html/template

### Architecture Pattern
Monolito ligero con binario unico autocontenido. El servidor Go sirve tanto la API como los assets estaticos. Sin dependencias externas de runtime.

---

## 2. Problem Statement

### Problem Being Solved
La Raspberry Pi "Ultron" corre multiples servicios (Docker containers y servicios Systemd) que necesitan monitoreo continuo. Actualmente no hay una forma centralizada de:
- Ver el estado de salud del sistema y todos sus servicios en un solo lugar
- Recibir alertas cuando algo falla o los recursos se acercan a limites criticos
- Controlar servicios (start/stop/restart) sin conectarse por SSH

### Target Users
- **Administrador unico:** El propietario de la Raspberry Pi, que necesita visibilidad y control remoto sobre sus servicios desde cualquier dispositivo en la red local o via VPN.

### Context
La Raspberry Pi tiene recursos limitados (tipicamente 1-4GB RAM, CPU ARM). El admin panel NO es el uso principal del dispositivo, por lo que debe consumir la menor cantidad de recursos posible sin sacrificar una experiencia de usuario profesional y moderna.

---

## 3. Feature Inventory

| Feature | Description | Status | Priority | Phase |
|---------|-------------|--------|----------|-------|
| FT-001: System Metrics Dashboard | Metricas en tiempo real de CPU, RAM, Disco, Red, Temperatura | Not Started | Must-have | 1 |
| FT-002: Docker Container Monitor | Estado, metricas y salud de todos los contenedores Docker | Not Started | Must-have | 1 |
| FT-003: Systemd Service Monitor | Estado de todos los servicios Systemd activos | Not Started | Must-have | 1 |
| FT-004: Alert System | Sistema de alertas con umbrales configurables | Not Started | Must-have | 1 |
| FT-005: Telegram Notifications | Envio de alertas criticas via Telegram Bot | Not Started | Must-have | 1 |
| FT-006: Email Notifications | Envio de alertas por email (SMTP configurable) | Not Started | Should-have | 1 |
| FT-007: Authentication | Login simple con usuario/password | Not Started | Must-have | 1 |
| FT-008: Service Controls | Start, Stop, Restart de servicios Docker y Systemd | Not Started | Must-have | 2 |
| FT-009: Dark Mode UI | Interfaz dark mode minimalista estilo Grafana/Portainer | Not Started | Must-have | 1 |

### Feature Details

#### FT-001: System Metrics Dashboard

**User Story:** As an admin, I want to see real-time system metrics on a single dashboard so that I can quickly assess the health of my Raspberry Pi.

**Acceptance Criteria:**
- [ ] Dashboard muestra uso de CPU en porcentaje con grafica historica (ultimos 60 min)
- [ ] Dashboard muestra uso de RAM (usado/total) con porcentaje y grafica
- [ ] Dashboard muestra uso de disco por particion (usado/total/porcentaje)
- [ ] Dashboard muestra trafico de red (bytes in/out por segundo) por interfaz
- [ ] Dashboard muestra temperatura del CPU en grados Celsius con indicador de color (verde < 60C, amarillo 60-75C, rojo > 75C)
- [ ] Metricas se actualizan automaticamente cada 5 segundos via SSE sin recargar pagina
- [ ] Dashboard muestra uptime del sistema

**Dependencies:** FT-007 (Auth), FT-009 (UI)
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-002: Docker Container Monitor

**User Story:** As an admin, I want to see the status of all Docker containers so that I know which services are running, stopped, or unhealthy.

**Acceptance Criteria:**
- [ ] Lista todos los contenedores Docker (running, stopped, exited)
- [ ] Muestra por contenedor: nombre, imagen, estado, uptime, CPU%, memoria usada
- [ ] Indicador visual de salud: verde (running), gris (stopped), rojo (exited con error)
- [ ] Los datos se actualizan automaticamente cada 10 segundos
- [ ] Click en un contenedor muestra detalles expandidos (ports, volumes, environment)

**Dependencies:** FT-007 (Auth), FT-009 (UI)
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-003: Systemd Service Monitor

**User Story:** As an admin, I want to see the status of Systemd services so that I can monitor background processes on my Raspberry Pi.

**Acceptance Criteria:**
- [ ] Lista todos los servicios Systemd activos/habilitados
- [ ] Muestra por servicio: nombre, estado (active/inactive/failed), desde cuando esta activo
- [ ] Indicador visual: verde (active), gris (inactive), rojo (failed)
- [ ] Filtro para mostrar solo servicios con errores
- [ ] Los datos se actualizan cada 30 segundos

**Dependencies:** FT-007 (Auth), FT-009 (UI)
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-004: Alert System

**User Story:** As an admin, I want to configure alerts with thresholds so that I get notified when something goes wrong before it becomes critical.

**Acceptance Criteria:**
- [ ] Umbrales configurables para: CPU > X%, RAM > X%, Disco > X%, Temperatura > X°C
- [ ] Alertas cuando un contenedor Docker cambia a estado unhealthy/exited
- [ ] Alertas cuando un servicio Systemd cambia a estado failed
- [ ] Historial de alertas persistido en SQLite con timestamp y tipo
- [ ] Panel de alertas en el dashboard con filtros por severidad (critical, warning, info)
- [ ] Las alertas tienen cooldown configurable para evitar spam (default: 15 min)
- [ ] Severidades: critical (rojo), warning (amarillo), info (azul)

**Dependencies:** FT-001, FT-002, FT-003
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-005: Telegram Notifications

**User Story:** As an admin, I want to receive critical alerts on Telegram so that I'm immediately aware of problems even when I'm not looking at the dashboard.

**Acceptance Criteria:**
- [ ] Configuracion de Telegram Bot Token y Chat ID desde el panel
- [ ] Envio de alertas critical y warning via Telegram
- [ ] Mensaje incluye: severidad, metrica/servicio afectado, valor actual, umbral configurado
- [ ] Boton de test para verificar la conexion con Telegram
- [ ] Opcion de silenciar notificaciones temporalmente (1h, 4h, 24h)

**Dependencies:** FT-004 (Alert System)
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-006: Email Notifications

**User Story:** As an admin, I want to receive alert summaries by email so that I have a record of all incidents.

**Acceptance Criteria:**
- [ ] Configuracion SMTP (host, port, user, password, from, to)
- [ ] Envio de emails para alertas critical
- [ ] Email incluye: resumen del problema, timestamp, valor actual vs umbral
- [ ] Opcion de enviar digest diario con resumen de alertas del dia
- [ ] Boton de test para verificar configuracion SMTP

**Dependencies:** FT-004 (Alert System)
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-007: Authentication

**User Story:** As an admin, I want to protect the panel with login credentials so that only authorized users can access it.

**Acceptance Criteria:**
- [ ] Pantalla de login con usuario y password
- [ ] Un unico usuario admin configurable (usuario/password se definen en config o primer setup)
- [ ] Sesion basada en cookie segura con expiracion configurable (default: 24h)
- [ ] Proteccion contra brute force: bloqueo temporal despues de 5 intentos fallidos (15 min)
- [ ] Password hasheado con bcrypt en la base de datos
- [ ] Boton de logout visible en el header
- [ ] Todas las rutas (excepto /login y /health) requieren autenticacion

**Dependencies:** Ninguna
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-008: Service Controls

**User Story:** As an admin, I want to start, stop, and restart services from the dashboard so that I don't need to SSH into the Raspberry Pi.

**Acceptance Criteria:**
- [ ] Botones de Start, Stop, Restart por cada contenedor Docker
- [ ] Botones de Start, Stop, Restart por cada servicio Systemd
- [ ] Confirmacion antes de ejecutar Stop/Restart (modal de confirmacion)
- [ ] Feedback visual del resultado de la accion (success/error con mensaje)
- [ ] Log de acciones ejecutadas con timestamp (audit trail)
- [ ] Las acciones se ejecutan de forma asincrona y reportan resultado

**Dependencies:** FT-002, FT-003, FT-007 (Auth)
**Status:** Not Started
**Confidence:** [HIGH]

---

#### FT-009: Dark Mode UI

**User Story:** As an admin, I want a professional dark mode interface so that the panel looks modern and is comfortable to use at any time.

**Acceptance Criteria:**
- [ ] Tema dark mode como default (fondo oscuro, texto claro)
- [ ] Paleta de colores coherente: grises oscuros (#1a1a2e, #16213e), acentos en azul/cyan
- [ ] Layout responsive: funcional en desktop, tablet y mobile
- [ ] Sidebar de navegacion colapsable
- [ ] Graficas con colores que contrasten en fondo oscuro
- [ ] Tipografia monospace para metricas, sans-serif para UI general
- [ ] Transiciones y animaciones sutiles (no distractoras)
- [ ] Iconografia consistente (preferir Heroicons o Lucide)

**Dependencies:** Ninguna
**Status:** Not Started
**Confidence:** [HIGH]

---

## 4. Functional Requirements

### Core Behaviours
- El servidor arranca como un unico binario Go, escuchando en un puerto configurable (default: 8080)
- Todas las metricas del sistema se recolectan internamente usando librerias Go nativas (gopsutil o similar)
- Los datos de Docker se obtienen via Docker socket (/var/run/docker.sock)
- Los datos de Systemd se obtienen via D-Bus o ejecutando `systemctl` commands
- Las metricas se emiten via Server-Sent Events (SSE) para actualizaciones en tiempo real
- HTMX maneja las interacciones del frontend sin necesidad de JavaScript framework

### Input/Output Specifications
- **Input:** Configuracion via archivo YAML o variables de entorno
- **Output:** Dashboard web HTML servido por el mismo binario Go
- **API:** Endpoints REST internos consumidos por HTMX (no API publica)

### Business Logic Rules
- Las alertas solo se disparan cuando se cruza un umbral (no reportar continuamente)
- Cooldown entre alertas del mismo tipo: configurable, default 15 minutos
- Los controles de servicio (Fase 2) requieren confirmacion explicita del usuario
- El historial de metricas se retiene por 24 horas (configurable), luego se purga automaticamente

---

## 5. Non-Functional Requirements

### Performance
- El binario completo debe usar menos de 30MB de RAM en operacion normal
- Tiempo de respuesta de pagina < 200ms en red local
- Recoleccion de metricas no debe consumir mas del 2% de CPU de forma sostenida
- El build debe producir un binario ARM autocontenido menor a 20MB

### Security
- Password hasheado con bcrypt (cost factor 10+)
- Cookies de sesion con flags: HttpOnly, Secure (si HTTPS), SameSite=Strict
- Proteccion CSRF en formularios
- Headers de seguridad: X-Content-Type-Options, X-Frame-Options, Content-Security-Policy
- No exponer informacion sensible en logs

### Scalability
- Disenado para un unico usuario/instancia (no multi-tenant)
- Debe funcionar correctamente monitoreando hasta 50 contenedores Docker y 100 servicios Systemd simultaneamente

### Availability
- El servicio debe arrancar automaticamente con el sistema (systemd unit file incluido)
- Debe recuperarse automaticamente de errores transitorios (reconexion a Docker socket, etc.)
- Health check endpoint (/health) para verificar que el servicio esta vivo

---

## 6. AI/ML Specifications

> No aplica para la version inicial. Posible integracion futura para analisis predictivo de metricas.

---

## 7. Data Architecture

### Data Models
- **User:** id, username, password_hash, created_at, last_login
- **Alert:** id, type (cpu/ram/disk/temp/docker/systemd), severity (critical/warning/info), message, value, threshold, created_at, acknowledged_at
- **AlertConfig:** id, metric, operator, threshold, severity, cooldown_minutes, enabled, notification_channels (json)
- **ActionLog:** id, user_id, action (start/stop/restart), target_type (docker/systemd), target_name, result (success/error), message, created_at
- **Session:** id, user_id, token, expires_at, created_at

### Relationships and Constraints
- Un User tiene muchas Sessions
- Un User tiene muchos ActionLogs
- AlertConfig es independiente (configuracion global)
- Alert es independiente (registro historico)

### Storage Mechanisms
- **SQLite** para datos persistentes (usuarios, alertas, configuracion, logs)
- **In-memory** para metricas en tiempo real (ring buffer de ultimas 24h)
- Archivo de base de datos en: `/var/lib/ultron-ap/ultron.db` (configurable)

---

## 8. Integration Map

### External Services
| Service | Purpose | Auth Method |
|---------|---------|-------------|
| Docker Engine | Monitoreo y control de contenedores | Unix socket (/var/run/docker.sock) |
| Systemd / D-Bus | Monitoreo y control de servicios | Acceso local (requiere permisos) |
| Telegram Bot API | Notificaciones push | Bot Token + Chat ID |
| SMTP Server | Notificaciones por email | SMTP auth (user/password) |

### Authentication Methods
- Login unico admin con username/password + cookie session

### Third-Party Dependencies
- **gopsutil:** Metricas del sistema (CPU, RAM, Disco, Red, Temperatura)
- **Docker SDK for Go:** Interaccion con Docker Engine
- **go-sqlite3 o modernc.org/sqlite:** Driver SQLite (pure Go preferido para cross-compile)
- **HTMX:** Interactividad frontend (CDN o embebido)
- **Tailwind CSS:** Estilos (build-time, CSS purgado embebido en binario)
- **golang.org/x/crypto/bcrypt:** Hashing de passwords

---

## 9. Configuration Reference

### Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `ULTRON_PORT` | Puerto del servidor web | No | `8080` |
| `ULTRON_DB_PATH` | Ruta al archivo SQLite | No | `/var/lib/ultron-ap/ultron.db` |
| `ULTRON_ADMIN_USER` | Usuario admin inicial | Yes (primer inicio) | `admin` |
| `ULTRON_ADMIN_PASS` | Password admin inicial | Yes (primer inicio) | - |
| `ULTRON_SESSION_TTL` | Duracion de sesion en horas | No | `24` |
| `ULTRON_TELEGRAM_TOKEN` | Token del Telegram Bot | No | - |
| `ULTRON_TELEGRAM_CHAT_ID` | Chat ID de Telegram | No | - |
| `ULTRON_SMTP_HOST` | Host del servidor SMTP | No | - |
| `ULTRON_SMTP_PORT` | Puerto SMTP | No | `587` |
| `ULTRON_SMTP_USER` | Usuario SMTP | No | - |
| `ULTRON_SMTP_PASS` | Password SMTP | No | - |
| `ULTRON_SMTP_FROM` | Email remitente | No | - |
| `ULTRON_SMTP_TO` | Email destinatario | No | - |
| `ULTRON_METRICS_INTERVAL` | Intervalo de recoleccion en segundos | No | `5` |
| `ULTRON_LOG_LEVEL` | Nivel de log (debug/info/warn/error) | No | `info` |

### Feature Flags
- Notificaciones Telegram y Email se habilitan automaticamente al configurar sus variables correspondientes

---

## 10. Quality Assessment

### Tested Functionality
- No hay tests aun (proyecto nuevo)

### Untested Areas
- Todo el proyecto esta por construir

### Technical Debt
- Ninguno (proyecto greenfield)

---

## 11. Open Questions

- **Q:** Deberia el panel soportar HTTPS nativo o delegarlo a un reverse proxy (Caddy/nginx)?
  **Context:** HTTPS es importante para seguridad de credenciales, pero agrega complejidad al binario.
  **Options:** (a) Soporte HTTPS nativo con auto-TLS, (b) HTTP only + reverse proxy, (c) Ambos opcionales

- **Q:** Se necesita retencion de metricas mas alla de 24h para graficas historicas?
  **Context:** Mas retencion = mas uso de disco/memoria. Podria exportarse a un servicio externo.
  **Options:** (a) 24h suficiente, (b) 7 dias, (c) Configurable con purga automatica

- **Q:** El primer setup (crear usuario admin) deberia ser via web wizard o solo via env vars?
  **Context:** Un wizard web es mas amigable pero agrega codigo para un flujo que solo se usa una vez.
  **Options:** (a) Solo env vars, (b) Web wizard en primer inicio, (c) CLI interactive

---

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-02-09 | 1.0.0 | PRD inicial creado - Dashboard de monitoreo para Raspberry Pi |

---

> **Confidence Markers:** [HIGH] clear from user input | [MEDIUM] inferred from context | [LOW] speculative
>
> **Status Values:** Complete | Partial | Stubbed | Broken | Not Started
