## Feature
Poner retención de 30 días a la tabla `NetSample`, que hoy crece sin límite y ocupa el 96% de `ultron.db`, y hacer configurable el intervalo del monitor de red.

## Problem / Why
Diagnóstico verificado en la Pi el 2026-09-06 sobre una copia de solo lectura de `/var/lib/ultron-ap/ultron.db`, y reconfirmado contra el código de este repo.

El archivo de base de datos pesa **719 MB** (+4 MB de WAL), con `journal_mode=wal`, `auto_vacuum=0`, `freelist_count=0` y `page_size` 4096. La tabla `NetSample` tiene **8.431.177 filas** desde el 2026-05-04, la primera muestra del monitor. Con sus índices `idx_net_sample_target_ts` e `idx_net_sample_ts` ocupa unos **694 MB, el 96% del archivo**.

El ritmo es de 5 objetivos (`1.1.1.1` icmp, `8.8.8.8` icmp, `gateway` icmp, `dns` dns, y a veces `192.168.1.1`/`1.1.1.1` dns) cada **5 s** — mediana de 5000 ms entre muestras consecutivas por objetivo. Son ~67.000 filas al día, unos 5,5 MB diarios.

La comparación con UPS deja claro que esto es un olvido, no una decisión: `ups_samples` tiene 265k filas y ~25 MB, muestrea cada 10 s, y su fila más vieja tiene 30,6 días. **La retención de UPS sí funciona**, vía `ULTRON_UPS_RETENTION_DAYS`, `(*Store).PruneSamples` y su llamada en `internal/ups/poller.go`.

Para red, la función `(*DB).PruneNetSamples` **ya existe** en `internal/database/network.go` con su `DELETE FROM NetSample WHERE ts < ?`, pero **nunca se llama desde ningún sitio fuera de su propio test**. Confirmado en el código: `startRetentionJob` en `internal/server/server.go` poda `ActionLog`/`Alert` con `PruneOldData(30)`, poda sesiones expiradas y poda UPS — y no toca `NetSample`. El log `retention: pruned %d records older than 30 days` que aparece en el journal es el de esas otras tablas, que están pequeñas; no cubre red. Tampoco existe ninguna variable de entorno de retención de red: las únicas `ULTRON_*` relacionadas son `ULTRON_NET_TARGETS` y `ULTRON_METRICS_INTERVAL`.

El intervalo de 5 s está fijo en código, en `cmd/ultron-ap/main.go` (`gatewayprobe.New(5*time.Second, ...)`), con el mismo valor como respaldo dentro de `gatewayprobe.New` cuando recibe un intervalo no positivo.

## Target Users
El operador de la Pi (César), único usuario del panel, que necesita que el dispositivo siga funcionando sin quedarse sin disco y que el histórico de red siga sirviendo para ver los cortes de la semana. Su dolor: una base de datos de 719 MB que crece 5,5 MB al día sin techo, en un dispositivo doméstico, y cuyos backups nocturnos cifrados a Drive arrastran ese peso cada noche.

## New Behavior
El sistema debe leer `ULTRON_NET_RETENTION_DAYS` del entorno, con valor por defecto 30 y mínimo 1, y usarlo como ventana de retención de `NetSample`.
El sistema debe rechazar un valor inválido o fuera de rango de esa variable, avisar en el log y seguir con el valor por defecto, sin abortar el arranque.
El sistema debe podar `NetSample` al arrancar y después cada 24 h, borrando las filas anteriores a la ventana de retención.
El sistema debe borrar en lotes acotados en vez de un único `DELETE` de millones de filas, para no bloquear el WAL en la primera pasada.
El sistema debe registrar en el log cuántas muestras podó y con qué ventana.
El sistema debe recuperar el espacio en disco tras podar, no limitarse a marcar las páginas como libres.
El sistema debe leer `ULTRON_NET_INTERVAL_SECONDS` del entorno, con valor por defecto 5 para no cambiar el comportamiento actual, y usarlo como intervalo del monitor de red.

## Success Criteria
Dada una base con muestras de 45 días y una ventana de 30, cuando corre la poda, entonces desaparecen las anteriores a 30 días y se conservan todas las posteriores.
Dada la variable sin definir, cuando arranca el proceso, entonces la ventana es de 30 días.
Dada la variable con un valor no numérico o menor que 1, cuando arranca el proceso, entonces se avisa en el log y la ventana es de 30 días.
Dada una base con millones de filas fuera de ventana, cuando corre la primera poda, entonces se borran en lotes sucesivos y ninguna sentencia individual abarca toda la tabla.
Dada una poda que liberó espacio, cuando termina, entonces el tamaño del archivo en disco baja, no solo el recuento de filas.
Dado `ULTRON_NET_INTERVAL_SECONDS` sin definir, cuando arranca el monitor de red, entonces su intervalo es de 5 s, idéntico al de hoy.
Dado el despliegue en la Pi con la ventana por defecto, cuando ha corrido la primera poda y su recuperación de espacio, entonces `ultron.db` ronda los 175 MB en vez de 719 MB, y el panel de cortes de red de la semana sigue mostrando datos.

## Touch Points
MODIFICA:
- `internal/server/server.go` — `startRetentionJob`, que es el planificador diario que ya poda ActionLog, sesiones y UPS; gana la poda de red.
- `internal/database/network.go` — `PruneNetSamples` pasa a borrar por lotes.
- `internal/database/sqlite.go` — configuración de `auto_vacuum` y recuperación de espacio.
- `cmd/ultron-ap/main.go` — el intervalo del `gatewayprobe` deja de estar fijo.
- `internal/config` — carga de las dos variables nuevas.
- `README.md` y `.env.example` — documentación de las dos variables.

NO TOCA:
- El esquema de `NetSample` ni los nombres de sus índices: los usa `RecentNetSamples` y la interfaz.
- La retención de UPS, que funciona.
- `ULTRON_NET_TARGETS`.

## Must Not Break (Regression Boundary)
La retención de UPS sigue funcionando exactamente igual, con su propia variable y su propia ventana.
El esquema de `NetSample` y los nombres `idx_net_sample_target_ts` e `idx_net_sample_ts` quedan intactos, porque `RecentNetSamples` y la interfaz dependen de ellos.
El panel de cortes de red de la semana sigue mostrando datos tras la poda: la ventana de 30 días cubre de sobra los 7 días que consulta.
La poda de `ActionLog`, `Alert` y sesiones expiradas sigue ocurriendo en el mismo trabajo diario y con la misma cadencia.
El monitor de red sigue muestreando cada 5 s cuando no se configura otra cosa, así que un despliegue sin cambiar el entorno no altera el comportamiento observable.
La inserción de muestras no se bloquea mientras corre la poda: el probe debe seguir escribiendo.

## Out of Scope
Cambiar el esquema de `NetSample`, sus índices o recrear la base.
Tocar la retención de UPS o `ULTRON_NET_TARGETS`.
Una interfaz para configurar la retención desde el panel: se configura por entorno, como la de UPS.
Agregación o downsampling del histórico de red, que sería otra feature con sus propias decisiones.
La vista Docker vía helper, que va en la feature `docker-via-helper`.
