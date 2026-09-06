## Feature
La vista de contenedores Docker deja de abrir `/var/run/docker.sock` desde la web app y pasa a pedirle los datos, en solo lectura, al helper privilegiado que ya corre como root.

## Problem / Why
Origen: auditoría de seguridad de la Pi del 2026-09-05, Bloque 2, hallazgo C2. Decisión del owner (César) el 2026-09-06: no revertir la Pi, corregir en el código.

La web app (`ultron-ap`, systemd `User=ultron`) abre `/var/run/docker.sock` directamente con el SDK de Docker (`internal/docker/monitor.go`, `NewMonitor()` → `dclient.NewClientWithOpts(dclient.FromEnv, ...)`). Para que eso funcionara, el usuario `ultron` estaba en el grupo `docker`.

Pertenecer al grupo `docker` equivale a ser root en la máquina: con acceso al socket se puede arrancar un contenedor que monte el sistema de archivos del host. La web app está expuesta en LAN (`:8080`) y en tailnet (`:34002`), así que cualquier ejecución remota de código en ella daba root en la Pi. El grupo se retiró en la Pi el 2026-09-06 a las 13:22 COT, y con eso el hallazgo quedó mitigado en producción pero el producto quedó degradado.

Efecto observado en el arranque siguiente:

```
ultron-ap: docker: daemon not reachable: permission denied while trying to connect to the Docker daemon socket at unix:///var/run/docker.sock
ultron-ap: Docker monitor started (interval=1m0s)
```

La sección de contenedores del panel queda vacía y sin explicación: el usuario no puede distinguir "no hay contenedores" de "no puedo leerlos". Todo lo demás (CPU, temperatura, red, UPS, alertas) sigue funcionando.

## Target Users
El owner de la Pi (César), único operador del panel, que usa la pestaña de contenedores para ver de un vistazo si `homeassistant`, `hermes`, `openclaw`, `ledger` y `mosquitto` están arriba y cuánta CPU y memoria consumen, desde el móvil en LAN o desde fuera por tailnet. Su dolor hoy: la sección está vacía y no sabe por qué. No se desbloquean nuevos tipos de usuario.

## New Behavior
El sistema debe leer la lista de contenedores (nombre, imagen, estado, texto de estado, uptime y health) a través del helper privilegiado, no del socket de Docker.
El sistema debe leer las stats por contenedor (porcentaje de CPU, memoria usada, límite de memoria y porcentaje de memoria) a través del helper.
El sistema debe leer el detalle de un contenedor (puertos, volúmenes y nombres de variables de entorno, nunca sus valores) a través del helper.
El sistema debe leer los logs de un contenedor a través del helper, aplicando la misma redacción que ya se aplica a los logs de journalctl.
El helper debe hablar con Docker por su socket usando la API HTTP mínima (`GET /containers/json?all=1`, `GET /containers/{id}/stats?stream=false`, `GET /containers/{id}/json`, `GET /containers/{id}/logs`) y nunca invocando el binario `docker`.
El helper debe rechazar cualquier operación de escritura sobre contenedores: no expone start, stop, restart ni exec, y una petición con una acción de ese tipo se responde como acción desconocida.
La web app no debe contener ninguna ruta de código que abra `/var/run/docker.sock`, y desaparece el log `daemon not reachable`.
Cuando el helper no responde o falla, el panel debe mostrar un estado explícito de "helper no disponible" en la sección de contenedores, en vez de mostrarla vacía.
Cada llamada de la web app al helper para datos de Docker debe estar acotada por un timeout corto de entre 2 y 3 segundos.
Los controles start, stop y restart de contenedores se retiran de la web app: handlers, rutas, botones de la interfaz y `internal/docker/controls.go`.

## Success Criteria
Dado que el usuario `ultron` NO pertenece al grupo `docker`, cuando se abre la pestaña de contenedores, entonces se listan los cinco contenedores de la Pi con su estado y sus stats.
Dado un arranque normal del servicio, cuando se inspecciona `journalctl -u ultron-ap`, entonces no aparece ningún `permission denied` ni ningún `daemon not reachable`.
Dado el helper detenido, cuando se abre la pestaña de contenedores, entonces la sección muestra "helper no disponible" y no una lista vacía.
Dado un helper que tarda más de 3 segundos, cuando la web app pide la lista, entonces la llamada se aborta por timeout y la sección cae al estado de "helper no disponible" sin bloquear el resto del panel.
Dada una petición al helper con acción de escritura sobre un contenedor, cuando el helper la procesa, entonces responde error de acción desconocida y no ejecuta nada.
Dado el árbol de código tras el cambio, cuando se busca `docker.sock` o el SDK de Docker en la web app, entonces no hay ninguna coincidencia fuera del helper.
Dado el detalle de un contenedor con variables de entorno, cuando se renderiza, entonces se muestran solo los nombres de las variables y ningún valor.

## Touch Points
MODIFICA:
- `internal/docker/monitor.go` — deja de crear un cliente del SDK de Docker; su fuente de datos pasa a ser el helper.
- `internal/docker/client.go` — la interfaz `DockerClient` pierde los métodos de escritura y de socket directo.
- `internal/privileged/client.go` — se le añaden los métodos cliente de las acciones nuevas de Docker.
- `cmd/ultron-helper/main.go` — `dispatch()` gana los casos de Docker, en solo lectura.
- `internal/server/handlers.go` — el handler de detalle de contenedor.
- `internal/server/server.go` — cableado del monitor.
- `cmd/ultron-ap/main.go` — construcción del monitor (`docker.NewMonitor()`).
- Las plantillas de la sección de contenedores, para el estado "helper no disponible" y para quitar los botones de control.

ELIMINA:
- `internal/docker/controls.go` y `internal/server/handlers_docker.go` (start/stop/restart), con sus rutas y sus tests.

AÑADE:
- El transporte HTTP mínimo contra el socket de Docker, dentro del helper.

## Must Not Break (Regression Boundary)
Las acciones existentes del helper (`ping`, `systemctl`, `logs`, `shutdown`) siguen respondiendo igual y con el mismo formato de `Response`.
La autenticación del helper por SO_PEERCRED sigue aplicándose a las acciones nuevas: un UID fuera de la allowlist sigue recibiendo `forbidden`, y el fail-closed sin allowlist (BG-043) se mantiene.
La redacción de logs (`internal/logfilter`) se sigue aplicando a todo lo que cruza la frontera del helper hacia la web app.
El resto del panel (CPU, temperatura, red, UPS, alertas, Tailscale) sigue renderizando aunque la sección de contenedores esté en estado de error.
El arranque de `ultron-ap` no falla ni se retrasa cuando el helper no está disponible.
Los controles de servicios de systemd de la interfaz (start/stop/restart de servicios, que NO son contenedores) siguen funcionando: lo que se retira es solo el control de contenedores Docker.

## Out of Scope
Control de contenedores desde la web app (start, stop, restart, exec) en cualquier forma. Si algún día se quiere, será una operación aparte con allowlist de nombres y confirmación explícita, y es una decisión que no se toma aquí.
La retención de `NetSample`, que va en la feature `net-sample-retention`.
El script de deploy con contraseña, que es cambio de tooling y documentación sin comportamiento de producto y va fuera del pipeline.
Devolver el usuario `ultron` al grupo `docker` bajo cualquier circunstancia.
