## Feature
Módulo UPS para Ultron AP: leer el UPS (Powest, USB, gestionado por NUT en la Pi) vía protocolo NUT nativo, mostrarlo en el dashboard, guardar historial de cortes y alertar por Telegram.

## Problem / Why
Ultron AP monitorea red, Docker, systemd y métricas del host, pero hoy no sabe nada de la energía que lo mantiene vivo. La Pi está protegida por un UPS conectado por USB y gestionado con NUT, y no hay visibilidad ni alertas sobre cortes de luz, estado de batería o autonomía del sistema. Alcance: **monitoreo, historial y alertas** — el apagado seguro lo sigue haciendo NUT (`nut-monitor`) y este módulo no lo reemplaza.

## Target Users
El operador/administrador de la infraestructura casera (César) que ya usa el dashboard de Ultron AP para vigilar la Pi. No abre nuevos tipos de usuario.

## New Behavior
- El sistema debe hablar el protocolo NUT por TCP contra `127.0.0.1:3493` en Go puro (paquete nuevo `internal/ups`), sin ejecutar `upsc` por `exec`, con usuario NUT de solo lectura propio de Ultron.
- El sistema debe sondear el UPS cada 10 s (configurable) y reconectar con backoff; "UPS inalcanzable" es un estado válido a mostrar, no un panic.
- El sistema debe mostrar una tarjeta de UPS en el dashboard con actualización en vivo por SSE: estado traducido (`OL`→En red, `OB`→En batería, `LB`→Batería baja, etc.), carga (%), voltaje de entrada, voltaje de batería + **% estimado** (siempre etiquetado "estimado"), estado del beeper, y "Sin datos" explícito cuando el UPS está inalcanzable.
- El sistema debe estimar el % de batería interpolando `battery.voltage` entre 21.0 V y 27.4 V (el UPS no entrega `battery.charge` ni `battery.runtime` ni `ups.realpower`).
- El sistema debe persistir muestras en SQLite (tabla `ups_samples`, reutilizando `internal/metrics`): series de `input.voltage`, `battery.voltage`, `ups.load` y estado, retención 30 días con purga, y gráficas 24 h / 7 d.
- El sistema debe registrar eventos de corte: cada transición a `OB` abre un evento y cada regreso a `OL` lo cierra, guardando inicio, fin y duración.
- El sistema debe integrarse con `internal/alerts` + `internal/notify` (Telegram) con antirrebote/deduplicación para: paso a batería (Warning), batería baja (Crítico), regreso de red (Info, con duración), `battery.voltage` cerca de 21.0 V (Crítico), voltaje de entrada fuera de ~100–140 V (Warning), `RB` (Warning 1×/día), UPS inalcanzable > 2 min (Warning 1×).
- El sistema debe alimentar el motor de insights existente (P2): conteo de cortes por semana, degradación de batería por caída del voltaje en reposo.
- El sistema debe mostrar (solo lectura) la configuración de apagado seguro del UPS/NUT: los delays del UPS (`ups.delay.shutdown`, `ups.delay.start`) y el umbral de batería con que NUT dispara el apagado, claramente etiquetados como "gestionado por NUT". No edita ni ejecuta nada de apagado.
- El sistema debe exponer como configurables (vía `/etc/ultron-ap/ultron-ap.env` / config del service): (a) los umbrales de alerta — rango de voltaje de entrada ~100–140 V, voltaje crítico de batería ~21.0 V, ventanas de antirrebote y timeout de "UPS inalcanzable"; (b) el intervalo de sondeo (default 10 s); (c) el rango de voltaje para el % estimado de batería (default 21.0–27.4 V); (d) la retención del historial (default 30 días).

## Success Criteria
- Given el UPS en red, When se abre el dashboard, Then muestra "En red", carga, voltajes y batería estimada, refrescándose solo por SSE.
- Given el UPS conectado, When se desconecta de la pared, Then en < 15 s el dashboard dice "En batería" y llega un mensaje de Telegram.
- Given un corte activo, When vuelve la red, Then llega un mensaje "red restablecida" con la duración del corte y el evento queda guardado en el historial.
- Given Ultron corriendo, When se para `nut-server`, Then el panel muestra "Sin datos", no se cae ni llena el log de errores.
- La estimación de batería aparece siempre marcada como estimada.
- `go test ./internal/ups/...` pasa.

## Touch Points
- **ADD:** paquete nuevo `internal/ups` (cliente NUT + parser + mapeo de estados), tarjeta nueva de UPS en el dashboard, tabla `ups_samples`.
- **MODIFY:** canal SSE existente (nueva tarjeta en vivo), `internal/metrics` (reutilizado para persistencia/purga), `internal/alerts` + `internal/notify` (nuevas reglas Telegram), motor de insights (nuevas señales), configuración del service (`ULTRON_UPS_ENABLED`, `ULTRON_NUT_USER`, `ULTRON_NUT_PASS`, más las claves de sondeo/umbrales/rango-batería/retención en `/etc/ultron-ap/ultron-ap.env`).

## Must Not Break (Regression Boundary)
- Si NUT no está instalado o el UPS no responde, Ultron AP arranca igual y la tarjeta se muestra deshabilitada (degradación limpia; el módulo se activa por config `ULTRON_UPS_ENABLED`).
- El módulo no requiere root ni privilegios nuevos ni tocar `ultron-helper` (salvo RF-6 opcional); las demás tarjetas y el canal SSE existente siguen funcionando igual.
- Todo valor que venga de NUT es entrada externa y debe escaparse antes de renderizar (no reintroducir el XSS de toasts vía `innerHTML`).
- El apagado seguro sigue siendo exclusivo de `nut-monitor`; el módulo tiene prohibido cualquier comando `shutdown.*` / `load.off`.

## Out of Scope
- Ejecutar o reemplazar el apagado seguro (lo hace `nut-monitor`).
- Mostrar autonomía restante en minutos (el hardware no la entrega).
- Soportar varios UPS o UPS de red (hay uno solo, por USB).
- Duplicar lo que ya muestra Home Assistant vía su integración NUT.
- RF-6 (silenciar beeper / test de batería vía `upscmd`) queda como P2 opcional, no comprometido en este alcance.
- **Editar** la configuración de apagado seguro desde el panel: cambiar los delays del UPS (`SET VAR` / usuario NUT de escritura) o el umbral de disparo de NUT (`upsmon.conf`). Se muestra en solo lectura; editarlo rompería RS-1 (solo lectura) y RS-2 (sin privilegios nuevos) y queda fuera de esta feature.
