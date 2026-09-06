# BUILD_PLAN — net-sample-retention

Generación: 1 (plan fresco). Ids `EP-01`..`EP-03` estables.

Increment pequeño y de una sola pieza: tres epics, no más. La granularidad viene del
producto, no del protocolo.

## EP-01 — Configuración de las dos variables   [status: done]
  Delivers:    US-096, US-100
  FRs:         FR-096, FR-100
  Makes pass:  TC-NSR-001h, TC-NSR-002e, TC-NSR-003f, TC-NSR-004f,
               TC-NSR-040h, TC-NSR-041e, TC-NSR-042f, TC-NSR-043e,
               TC-NSR-100h, TC-NSR-101e, TC-NSR-102f
  Build steps: skeleton (campos NetRetentionDays y NetInterval en config.Config)
               → integraciones (carga con aviso y defecto, patrón de internal/ups)
               → hardening (saneo antes de salir de Load, cableado del probe en main.go)
  Why here:    Todo lo demás consume estos dos valores. Y es donde vive el control de
               seguridad de NFR-102: sanear en Load, no en el punto de uso.

## EP-02 — Poda por lotes y recuperación de espacio   [status: done]
  Delivers:    US-097, US-098, US-099
  FRs:         FR-097, FR-098, FR-099
  Makes pass:  TC-NSR-010h, TC-NSR-011e, TC-NSR-012e, TC-NSR-013e, TC-NSR-014f,
               TC-NSR-020h, TC-NSR-021e, TC-NSR-022f, TC-NSR-023e,
               TC-NSR-030h, TC-NSR-031e, TC-NSR-032e, TC-NSR-033e, TC-NSR-034f,
               TC-NSR-050h, TC-NSR-051f, TC-NSR-052f, TC-NSR-053e
  Build steps: skeleton (PruneNetSamples por lotes, FreeSpaceBytes, Compact)
               → integraciones (cableado en startRetentionJob tras la poda de UPS)
               → hardening (umbral de compactación, contención de errores, parámetros enlazados)
  Why here:    Es el corazón de la feature y necesita EP-01 para saber qué ventana aplicar.

## EP-03 — Regresión y documentación   [status: done]
  Delivers:    US-097 (frontera de no-romper)
  FRs:         —
  Makes pass:  TC-NSR-060h, TC-NSR-061f, TC-NSR-062e,
               TC-NSR-070h, TC-NSR-071e, TC-NSR-072f,
               TC-NSR-080h, TC-NSR-081e, TC-NSR-082f,
               TC-NSR-090h, TC-NSR-091e, TC-NSR-092f,
               TC-NSR-110h, TC-NSR-111e, TC-NSR-112f
  Build steps: skeleton (tests de las seis NFR de regresión)
               → integraciones (README y .env.example con las dos variables)
               → hardening (concurrencia poda/inserción, esquema e índices intactos)
  Why here:    Va al final porque su trabajo es demostrar que EP-01 y EP-02 no rompieron
               nada: la retención de UPS, el esquema, el panel semanal, las otras podas,
               el muestreo por defecto y la escritura del probe.

## Cierre — evidencia final

- 45 tests TC-NSR en verde (los 44 TC declarados mas un test auxiliar de silencio del log).
  Suite completa del repo: 46 paquetes ok, cero fallos. go vet limpio.
- **Un fallo real que encontro el test, no la revision:** TC-NSR-030h fallo con
  "5890048 is not less than 5890048". Causa: la base corre en modo WAL, donde VACUUM escribe
  el contenido reconstruido en el write-ahead log y deja el archivo principal en su tamano
  anterior hasta que un checkpoint lo pliega. Tal como estaba escrito, Compact() habria
  ejecutado un VACUUM con exito en la Pi y `ls -la ultron.db` habria seguido marcando 719 MB:
  las filas borradas, el disco no devuelto, y FR-099 incumplido en silencio. Arreglado
  anadiendo PRAGMA wal_checkpoint(TRUNCATE) despues del VACUUM.
- El test preexistente TestPruneNetSamples sigue pasando sin tocarlo: la firma de
  PruneNetSamples se conservo a proposito y el cambio a lotes es invisible para el llamante.
- Los TC de NFR-102 (TC-NSR-051f y 052f) se escribieron de extremo a extremo —
  entorno, config.Load, poda, filas— en vez de solo comprobar que Load devuelve 30. Afirmar
  el saneador aisladamente habria dejado sin verificar lo que de verdad importa: que ningun
  valor destructivo sobrevive el viaje hasta la base.
