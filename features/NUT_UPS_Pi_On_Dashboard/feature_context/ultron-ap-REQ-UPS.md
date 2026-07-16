# Requerimiento — Módulo UPS para Ultron AP

**Fecha:** 2026-07-14
**Autor:** César (vía Claude en la Pi)
**Estado:** Propuesta / pendiente de aprobación
**Repo destino:** `github.com/cesareyeserrano/ultron-ap` (implementación en el Mac)

---

## 1. Objetivo

Ultron AP monitorea red, Docker, systemd y métricas del host, pero hoy **no sabe nada de la energía que lo mantiene vivo**. La Pi está protegida por un UPS conectado por USB y gestionado con NUT (Network UPS Tools), que ya corre en el host. El requerimiento es que Ultron AP lea ese UPS, lo muestre en el panel, y avise cuando algo pase.

Alcance: **monitoreo, historial y alertas.** El apagado seguro en caso de batería crítica lo sigue haciendo NUT (`nut-monitor`), y este módulo **no debe intentar reemplazarlo**.

---

## 2. Realidad del hardware (verificado en la Pi el 2026-07-14)

Esto no es teoría: es la salida real de `upsc powest`.

| Dato | Valor |
|---|---|
| UPS | Powest, tipo `offline / line interactive` |
| Conexión | USB — Cypress Semiconductor USB-to-Serial, VID `0665` / PID `5161` |
| Driver NUT | `nutdrv_qx` (protocolo Q1/Megatec), NUT 2.8.1 |
| Nombre NUT | `powest` |
| Servidor | `upsd` escuchando en TCP **3493** (localhost + `172.17.0.1` para Docker) |
| Batería | nominal 24 V; rango útil configurado: **21.0 V (baja) — 27.4 V (alta)** |

### Variables que el UPS SÍ entrega

`ups.status` (ej. `OL`), `ups.load` (%), `input.voltage`, `input.frequency`, `output.voltage`, `battery.voltage`, `ups.beeper.status`, `ups.delay.shutdown`, `ups.delay.start`, `ups.type`.

### Variables que el UPS **NO** entrega — restricción de diseño

**No hay `battery.charge` (% de batería) ni `battery.runtime` (autonomía restante) ni `ups.realpower`.** Es un UPS barato de protocolo Q1 y simplemente no los publica.

Consecuencias, y hay que asumirlas explícitamente en la implementación:

1. El "% de batería" **debe estimarse** interpolando `battery.voltage` entre 21.0 V y 27.4 V. Debe mostrarse siempre etiquetado como **estimado**, nunca como dato del equipo.
2. La autonomía restante en minutos **no se puede mostrar** de forma honesta. Se descarta del alcance. (Si algún día se quiere, habría que calibrarla midiendo una descarga real y modelando la curva contra `ups.load`; eso es otro requerimiento.)
3. La curva voltaje→carga de plomo-ácido no es lineal, así que la estimación es orientativa. Sirve para "va bien / va a la mitad / se está acabando", no para dar un número exacto.

---

## 3. Requerimientos funcionales

### RF-1 — Cliente NUT nativo (P0)

Nuevo paquete `internal/ups`. Habla el **protocolo NUT por TCP contra `127.0.0.1:3493`** (`LIST VAR powest` / `GET VAR`), en Go puro.

- **No** ejecutar el binario `upsc` por `exec`. `ultron-ap` corre con `ProtectSystem=full` y `NoNewPrivileges`; un socket TCP es más limpio, más testeable y no necesita el helper privilegiado.
- Autenticación: usuario NUT **de solo lectura propio de Ultron** (ver RS-1). No reutilizar el usuario `homeassistant` que ya existe — cada consumidor con su credencial.
- Sondeo cada 10 s (configurable). Reconexión con backoff si `upsd` se cae; el UPS no disponible es un estado válido a mostrar, no un panic.

### RF-2 — Tarjeta de UPS en el dashboard (P0)

Con actualización en vivo por el mismo canal **SSE** que ya usan las otras tarjetas:

- Estado grande y legible traducido desde `ups.status`: `OL` = En red, `OB` = **En batería**, `LB` = **Batería baja**, `OL CHRG` = Cargando, `RB` = Reemplazar batería, `OFF`, `BYPASS`, `ALARM`.
- Carga del UPS (`ups.load` %) — hoy 2 %, o sea la Pi va sobradísima.
- Voltaje de entrada (detecta bajones de la red antes de que corten).
- Voltaje de batería + **% estimado** (con la etiqueta "estimado" visible, según la restricción de arriba).
- Estado del beeper.
- Cuando el UPS está inalcanzable: estado "Sin datos" explícito, no una tarjeta en blanco ni ceros.

### RF-3 — Historial y gráficas (P1)

Persistir muestras en la SQLite que ya usa Ultron (tabla `ups_samples`), reutilizando lo que ya hace `internal/metrics`.

- Series: `input.voltage`, `battery.voltage`, `ups.load`, y estado.
- Retención por defecto 30 días, con purga automática.
- Gráfica de las últimas 24 h / 7 d.
- **Registro de eventos de corte:** cada transición a `OB` abre un evento, cada regreso a `OL` lo cierra, guardando inicio, fin y duración. Esto es lo que de verdad se quiere responder: *"¿cuántas veces se fue la luz este mes y por cuánto tiempo?"*.

### RF-4 — Alertas (P1)

Integrarse con el motor de `internal/alerts` + `internal/notify` (Telegram) que ya existe. Reglas mínimas:

| Evento | Severidad | Aviso |
|---|---|---|
| Pasa a batería (`OB`) | Warning | Inmediato |
| Batería baja (`LB`) | Crítico | Inmediato |
| Vuelve la red (`OL`) | Info | Inmediato, con la duración del corte |
| `battery.voltage` cerca de 21.0 V | Crítico | Inmediato |
| Voltaje de entrada fuera de ~100–140 V | Warning | Con antirrebote |
| `RB` (reemplazar batería) | Warning | Una vez al día, no spam |
| UPS inalcanzable > 2 min | Warning | Una vez |

Todas con antirrebote/deduplicación: un parpadeo de la red no puede disparar diez mensajes.

### RF-5 — Insights (P2)

Alimentar el motor de insights ya existente: "la red eléctrica tuvo 4 cortes esta semana", "el voltaje de batería en reposo bajó de 27.4 V a 26.1 V en dos meses → la batería se está degradando".

### RF-6 — Acciones sobre el UPS (P2, opcional)

Solo si se quiere: silenciar/activar el beeper (`beeper.mute`, `beeper.enable`) y prueba de batería (`test.battery.start.quick`) vía `upscmd`, con usuario NUT con permisos de comando distinto del de lectura, expuesto a través de `ultron-helper`.

**Prohibido en este módulo:** cualquier comando de apagado (`shutdown.*`, `load.off`). El apagado seguro es de `nut-monitor` y punto. Un bug en el panel no puede apagar la casa.

---

## 4. Requerimientos no funcionales / seguridad

- **RS-1 — Credencial propia.** Crear en `/etc/nut/upsd.users` un usuario dedicado (ej. `ultron`) con permisos de **solo lectura**, contraseña propia. No reutilizar la de `homeassistant`. La contraseña va en `/etc/ultron-ap/ultron-ap.env` (`ULTRON_NUT_USER`, `ULTRON_NUT_PASS`), que es lo que ya carga el service, y se cifra en reposo con el `ULTRON_SECRET_KEY` existente si se guarda en DB. **Nunca en el repo ni por chat.**
- **RS-2 — Sin privilegios nuevos.** El módulo no necesita root ni tocar `ultron-helper` (salvo RF-6, que es opcional).
- **RS-3 — Degradación limpia.** Si NUT no está instalado o el UPS no responde, Ultron AP arranca igual y la tarjeta se muestra deshabilitada. El módulo se activa por config (`ULTRON_UPS_ENABLED`).
- **RS-4 — Escapado.** Todo valor que venga de NUT es entrada externa: escaparlo antes de renderizar (recordar el bug de XSS en toasts vía `innerHTML`).
- **RS-5 — Tests.** Cubrir el parser del protocolo NUT y el mapeo de estados con un `upsd` simulado, como ya se hace en el resto del repo.

---

## 5. Fuera de alcance

- Ejecutar o reemplazar el apagado seguro (lo hace `nut-monitor`).
- Mostrar autonomía restante en minutos (el hardware no la da).
- Soportar varios UPS o UPS de red. Hay uno, por USB.
- Duplicar lo que ya muestra Home Assistant (que ya tiene la integración NUT). Ultron AP lo quiere para **su** vista de infraestructura y sus alertas.

---

## 6. Criterios de aceptación

1. Con el UPS en red, el dashboard muestra estado "En red", carga, voltajes y batería estimada, y se refresca solo por SSE.
2. Al desconectar el UPS de la pared, en menos de 15 s el dashboard dice "En batería" y llega un mensaje de Telegram.
3. Al reconectar, llega un mensaje de "red restablecida" con la duración del corte, y el evento queda guardado en el historial.
4. Si se para `nut-server`, el panel muestra "Sin datos" y no se cae ni llena el log de errores.
5. La estimación de batería aparece siempre marcada como estimada.
6. `go test ./internal/ups/...` pasa.

---

## 7. Orden sugerido

**Fase 1 (P0):** cliente NUT + tarjeta en vivo → ya resuelve el 80 % del valor.
**Fase 2 (P1):** persistencia, eventos de corte, alertas por Telegram.
**Fase 3 (P2):** insights, y beeper/test si se quiere.
