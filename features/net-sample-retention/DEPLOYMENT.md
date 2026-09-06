# DEPLOYMENT — net-sample-retention

**Modelo de despliegue: binarios nativos.** Sin contenedores. Solo cambia `ultron-ap`;
`ultron-helper` no se toca en esta feature — pero sí en `docker-via-helper`, y ambas se
despliegan en el mismo ciclo, así que en la práctica van los dos binarios.

## Prerrequisitos

- Espacio libre en el NVMe equivalente al tamaño actual de `ultron.db` (~719 MB) para el
  `VACUUM`. En la Pi sobra con mucho.
- **Backup previo.** Ya existe `ultron.db.bak-network-rules-20260511-212401` de 33 MB, pero es
  de mayo. Conviene otro antes del primer arranque:
  ```sh
  ssh -t cesareyeserrano@192.168.1.29 'sudo cp /var/lib/ultron-ap/ultron.db /var/lib/ultron-ap/ultron.db.bak-pre-retention-$(date +%Y%m%d-%H%M%S)'
  ```
  El backup cifrado nocturno de las 03:30 a Drive también cubre `/var/lib/ultron-ap`, pero
  este cambio borra 6,3 millones de filas de forma irreversible: conviene tener uno propio y
  reciente antes, no confiar en el de anoche.

## Configuración

Añadir a `/etc/ultron-ap/ultron-ap.env` (root, 600) **antes** de reiniciar:

```
ULTRON_NET_RETENTION_DAYS=30
ULTRON_NET_INTERVAL_SECONDS=15
```

**Ambas son opcionales.** Sin ellas los valores son 30 y 5, y el comportamiento de muestreo
es idéntico al actual. El `15` es tu decisión para dividir el crecimiento entre tres.

## Construcción y despliegue

```sh
make build-arm
```

El procedimiento de copia e instalación es el mismo que documenta
`features/docker-via-helper/DEPLOYMENT.md`, con un solo `sudo` sobre `ssh -t`. Las dos
features se despliegan juntas.

## Qué ocurre en el primer arranque

**Un minuto después de arrancar**, no durante el arranque, el trabajo de retención:

1. Borra ~6,3 millones de filas de `NetSample` en lotes de 50.000 — unas 126 sentencias.
   El panel sigue respondiendo y el probe sigue insertando entre lote y lote.
2. Comprueba el espacio recuperable. Tras esa poda rondará los 540 MB, muy por encima del
   umbral de 100 MB, así que compacta.
3. Ejecuta `VACUUM` seguido de `PRAGMA wal_checkpoint(TRUNCATE)`. **Durante esta parte las
   escrituras esperan**, unos segundos sobre la base ya podada. Se pueden perder algunas
   muestras de red aisladas si el `busy_timeout` expira; es un coste único y aceptado.

Previsión total: **2-3 minutos** con el servicio operativo.

Es la única vez que esto ocurre. En régimen estacionario, cada poda diaria borra
aproximadamente lo que se insertó hace 30 días —un par de lotes— y el umbral de compactación
no vuelve a alcanzarse, porque SQLite reutiliza las páginas que la poda libera.

## Verificación posterior

```sh
ssh cesareyeserrano@192.168.1.29 'ls -la /var/lib/ultron-ap/ultron.db'
```

Debe rondar los **175 MB**, frente a los 719 MB de partida. Con
`ULTRON_NET_INTERVAL_SECONDS=15` bajará luego hacia ~58 MB a medida que el histórico se
renueve al ritmo nuevo.

```sh
ssh cesareyeserrano@192.168.1.29 'journalctl -u ultron-ap -n 100 --no-pager | grep retention'
```

Debe mostrar la línea de poda con el número de filas y la ventana, y la de compactación.

En el navegador: **"Cortes de red esta semana" debe seguir mostrando datos.** Una ventana de
30 días cubre de sobra los 7 que consulta ese panel; si sale vacío, algo va mal.

## Health checks

Sin cambios: `GET /health` sigue devolviendo 200 mientras el proceso vive. Esta feature no
añade endpoints ni servicios.

## Rollback

El binario anterior queda como `.prev` (ver el procedimiento de la otra feature). Pero
conviene entender qué revierte y qué no:

- **Revertir el binario detiene la poda futura**, así que la tabla vuelve a crecer sin techo.
- **NO devuelve las filas borradas.** Son 6,3 millones de muestras de más de 30 días y se
  fueron para siempre. Recuperarlas exige restaurar el backup previo.
- **NO deshace la compactación**, que de todos modos no es una pérdida: el archivo compactado
  contiene exactamente los mismos datos.

Si lo que se quiere es solo dejar de podar sin revertir el binario, basta con poner una
ventana muy amplia:

```
ULTRON_NET_RETENTION_DAYS=3650
```

Es preferible al rollback: conserva el resto de la feature y es reversible.
