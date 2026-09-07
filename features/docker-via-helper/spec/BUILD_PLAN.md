# BUILD_PLAN — docker-via-helper

Generación: 1 (plan fresco, primera entrada a fase 4). Ids `EP-01`..`EP-05` asignados una vez y estables.

Orden por dependencia: el transporte contra Docker debe existir antes de que el helper pueda
servir nada; el helper debe servir antes de que el monitor pueda consumir; el monitor debe
entregar disponibilidad antes de que la plantilla pueda distinguir estados. La retirada de los
controles y la limpieza de dependencias van al final, cuando ya no hay nada que dependa del SDK.

Los 59 TCs de `03_TEST_CASES.json` están repartidos: cada uno aparece en el `Makes pass` de
exactamente un epic.

## EP-01 — Transporte HTTP contra el socket de Docker   [status: done]
  Delivers:    US-089, US-090
  FRs:         FR-089, FR-090
  Makes pass:  TC-DVH-013f, TC-DVH-014f, TC-DVH-015f, TC-DVH-016e, TC-DVH-020h, TC-DVH-021f,
               TC-DVH-022e, TC-DVH-023e, TC-DVH-024f, TC-DVH-074e
  Evidencia:   go test ./internal/dockerapi/ -run TestTC_DVH -v → 10/10 PASS (0.379s).
               Reajuste respecto al plan inicial: TC-DVH-010h/011f/012f prueban el dispatch del
               helper, no el transporte, y se mueven a EP-02; TC-DVH-074e (desmultiplexado) baja
               aqui, que es donde vive el codigo. El reparto sigue siendo 59 TCs, uno por epic.
  Build steps: skeleton (`internal/dockerapi`: cliente, validación de id, tipos estrechos)
               → integraciones (Containers, Stats, Inspect, Logs contra un demonio simulado)
               → hardening (io.LimitReader, GET literal, descarte de valores de entorno)
  Why here:    Es la base de la que cuelga todo lo demás y la que concentra los controles de
               seguridad. Nada puede probarse antes de que exista.

## EP-02 — Acciones de Docker en el helper   [status: done]
  Delivers:    US-089, US-095
  FRs:         FR-089, FR-095
  Makes pass:  TC-DVH-010h, TC-DVH-011f, TC-DVH-012f,
               TC-DVH-070h, TC-DVH-071f, TC-DVH-072e, TC-DVH-073f,
               TC-DVH-080h, TC-DVH-081f, TC-DVH-082f, TC-DVH-083e,
               TC-DVH-090h, TC-DVH-091f, TC-DVH-092e,
               TC-DVH-100h, TC-DVH-101f, TC-DVH-102e
  Build steps: skeleton (tres casos nuevos en `dispatch`)
               → integraciones (desmultiplexado del flujo de logs, `finalizeLog` compartido)
               → hardening (regresión de las acciones existentes, peercred, redacción idéntica)
  Why here:    Necesita EP-01. Cierra la mitad privilegiada antes de tocar la web app, para que
               el consumidor se escriba contra un servidor que ya funciona.
  Evidencia:   go test ./cmd/ultron-helper/ -run TestTC_DVH -v → 17/17 PASS (0.5s), suite completa
               del paquete verde. Refactor necesario: la decision de autenticacion de handleConn se
               extrajo a authorize(), una funcion pura. Motivo: SO_PEERCRED es solo de Linux (en
               darwin peerCredSupported=false y handleConn se lo salta), y ni siquiera en Linux
               puede un test conectar como otro UID sin ser root, asi que un round trip cruzando
               UIDs no es alcanzable desde ningun entorno de test de este proyecto. TC-DVH-081f y
               082f verifican la decision real que toma produccion; TC-DVH-080h si hace el round
               trip completo por socket con el UID del proceso. Declarado en technical_debt.

## EP-03 — El monitor consume el helper   [status: done]
  Delivers:    US-088, US-092
  FRs:         FR-088, FR-092
  Makes pass:  TC-DVH-001h, TC-DVH-002e, TC-DVH-003e, TC-DVH-004e, TC-DVH-005f, TC-DVH-006e,
               TC-DVH-040h, TC-DVH-041f, TC-DVH-042e, TC-DVH-043e
  Build steps: skeleton (`containerSource`, `helperSource`, métodos cliente en `privileged`)
               → integraciones (`refresh` sobre la nueva fuente, conservar última lista buena)
               → hardening (timeout de 3s, recuperación tras timeout, sin fugas de goroutines)
  Why here:    Necesita EP-02 sirviendo. Es el punto donde el SDK deja de usarse de hecho,
               aunque todavía no se haya borrado.
  Evidencia:   go test ./... → 45 paquetes ok, cero fallos. Los 10 TC del epic en verde.
               Ajuste de alcance forzado por el compilador: reescribir el monitor obliga a borrar
               controls.go, client.go, los handlers de accion y sus rutas EN EL MISMO PASO, porque
               Go no compila si el codigo viejo y el nuevo coexisten. Ese borrado estaba planificado
               en EP-04/EP-05; se adelanta aqui por necesidad tecnica, no por cambio de alcance.
               Los TC siguen asignados a sus epics originales.
               Migracion de tests preexistentes (sin marcador @aitri-tc, por tanto no acreditados
               pero si cobertura real): monitor_mock_test.go borrado (mock del SDK, colaborador que
               ya no existe); monitor_test.go y monitor_detail_test.go reescritos contra fakeSource;
               los tests de calculateCPUPercent se movieron a internal/dockerapi con el codigo.
               TC-002b de la raiz (auto-refresh cada 10s), que SI esta acreditado, sigue pasando.
               Se borraron los tests de start/stop/restart de contenedor: prueban funcionalidad
               retirada a proposito, y su sustituto son TC-DVH-060h/061f/062f, que afirman la
               ausencia de esas rutas.

## EP-04 — Estados de la pantalla y retirada de los controles   [status: done]
  Delivers:    US-091, US-094
  FRs:         FR-091, FR-094
  Makes pass:  TC-DVH-030h, TC-DVH-031f, TC-DVH-032f, TC-DVH-033e, TC-DVH-034e, TC-DVH-035h,
               TC-DVH-036h, TC-DVH-060h, TC-DVH-061f, TC-DVH-062f, TC-DVH-063e,
               TC-DVH-110h, TC-DVH-111f, TC-DVH-112e, TC-DVH-120h, TC-DVH-121f, TC-DVH-122e
  Build steps: skeleton (`dockerPageData.Available` hasta el parcial)
               → integraciones (condicional raíz de la lista, borrado de handlers y rutas)
               → hardening (contraste de tokens, regresión de systemd y del resto del panel)
  Why here:    Necesita EP-03 entregando disponibilidad real. Aquí se materializa lo que ve
               el operador y desaparecen los controles.
  Evidencia:   go test ./internal/server/ -run TestTC_DVH -v → 17/17 PASS. Suite completa del repo:
               45 paquetes ok, cero fallos. 53/59 TCs de la feature en verde (faltan los 5 de EP-05
               y ninguno mas).
               Dos aserciones mal planteadas que corregi tras verlas fallar, no ajustando el codigo
               de produccion sino el test, porque el codigo tenia razon:
               (1) TC-DVH-111f afirmaba que un nombre de unidad invalido NO devuelve 200. Falso:
                   renderServicesResult devuelve 200 a proposito para que HTMX intercambie un
                   banner de error en vez de texto crudo. La propiedad de seguridad real es que no
                   se ejecute ningun systemctl, asi que se anadio registro de invocaciones al
                   mockCommandRunner (aditivo) y se afirma sobre eso.
               (2) TC-DVH-121f incluia /services, pero el fixture de docker no cablea monitor de
                   systemd y la ruta entraba en nil pointer. Probar eso seria probar el fixture;
                   /services se cubre en TC-DVH-110h con su propio fixture.
               Cambio de infraestructura: shortID se registro en pageFuncs (antes solo estaba en
               partialFuncs) porque la fila de docker-list.html gano su propio destino de detalle
               al perder la columna de controles. CSS reconstruido con make css.
               Nota sobre -race: go test -race ./... falla en TestSystemReader_ReadCPU
               (internal/metrics, 'host_processor_info returned nil cpuload'). Verificado
               PREEXISTENTE en un worktree limpio de HEAD ce87962, y ajeno a esta feature: es un
               flake de macOS bajo el detector de carreras con la suite entera en paralelo; pasa
               3/3 al correr el paquete aislado. CI corre en ubuntu-latest, donde ese camino de
               codigo (darwin) no se compila siquiera.

## EP-05 — Retirada del SDK y limpieza del gate de seguridad   [status: done]
  Delivers:    US-093
  FRs:         FR-093
  Makes pass:  TC-DVH-050h, TC-DVH-051e, TC-DVH-052f, TC-DVH-053f, TC-DVH-054e
  Build steps: skeleton (borrar `controls.go`, `client.go` y los imports del SDK)
               → integraciones (`go mod tidy`, vaciar `.github/vuln-allowlist.txt`)
               → hardening (test de grafo de dependencias con su control positivo)
  Why here:    Va el último a propósito: solo se puede borrar la dependencia cuando ningún
               epic anterior la usa. Incluye el vaciado obligatorio de la allowlist, porque el
               gate de seguridad falla tanto por vulnerabilidad nueva como por entrada que ya
               no se reporta.

## Cierre — evidencia final

- 59/59 TC de la feature en verde. Suite completa del repo: 46 paquetes ok, cero fallos. go vet limpio.
- go.mod: 59 -> 37 lineas. go.sum: -115 lineas. Cero entradas docker/moby/opencontainers.
- govulncheck verificado localmente (v1.7.0, db 2026-09-02): "Reachable: none", y GO-2026-4887 y
  GO-2026-4883 aparecen CERO veces en el escaneo. check-vulns.sh contra la allowlist vaciada sale 0.
  La afirmacion central del diseno queda comprobada empiricamente, no solo argumentada.
- Compilacion cruzada linux/arm64 de los dos binarios: OK.
- Higiene encontrada por el camino: web/static/.DS_Store y web/templates/.DS_Store existian en el
  arbol de trabajo (ignorados por git, por eso no estaban en HEAD) y ambos directorios van por
  go:embed. Eliminados; era el hallazgo RQ-SEC-004 del gate de seguridad.
- security-gate.sh sigue con 2 fallos PREEXISTENTES, verificados en worktree limpio de HEAD ce87962
  y ajenos a esta feature: RQ-SEC-002 (deploy/ultron-ap.sudoers con comodin pironman5) y RQ-SEC-005
  (ids de traza Aitri en JS de cliente). No se declaran como quality_gate de esta feature porque
  fallarian por deuda que no introduce este trabajo. Pendiente de registrar aparte.
