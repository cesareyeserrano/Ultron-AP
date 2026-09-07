# 01_UX_SPEC — docker-via-helper

Alcance visual de esta feature: la pantalla **Docker** (`/docker`) y sus dos parciales
(lista de contenedores y detalle de contenedor). No se toca ninguna otra pantalla.

El cambio de producto es de seguridad, no de estilo. Por tanto esta especificación
**no introduce ningún token nuevo ni ningún componente nuevo**: reutiliza el sistema
de diseño aprobado en la raíz. Lo que sí cambia es *qué estado se muestra cuándo*, y
la desaparición de una zona de acción.

> Nota de implementación: este documento describe los estilos por su **rol y token**,
> nunca escribiendo el nombre literal de una utilidad de Tailwind. Tailwind v4 escanea
> el árbol del proyecto entero además de sus `@source`, así que nombrar una utilidad en
> un `.md` la mete en el bundle compilado. Es exactamente el defecto que costó 220 bytes
> muertos en `app.css` y que se corrigió en el commit `84dec6a`.

---

## Defecto de UX que origina el trabajo

Hoy los dos estados conviven y se contradicen. `docker.html` pinta un distintivo
"Docker unavailable" cuando `.Content.Available` es falso, pero `partials/docker-list.html`
decide su contenido **solo** por si la lista está vacía — así que en el mismo render el
usuario ve a la vez "Docker unavailable" arriba y el estado vacío con el texto
"No containers found" y un icono de caja abajo.

El resultado es el peor de los dos mundos: el mensaje dominante y central dice que no hay
contenedores, que es falso, y el que dice la verdad es un distintivo pequeño en la esquina.
Tras retirar `ultron` del grupo `docker` el 2026-09-06, ese es precisamente el estado que
ve el operador en la Pi.

**Regla que esta feature establece: disponibilidad y vacuidad son estados excluyentes.**
La lista decide primero si hay fuente de datos, y solo si la hay decide si está vacía.

---

## User Flows

Persona única: **Raspberry Pi Operator** (definida en la raíz; sin cambios).

### Flujo A — Consultar el estado de los contenedores (camino feliz)

- **Entrada:** el operador abre `/docker` desde el menú lateral, en móvil por LAN o por tailnet.
- **Pasos:**
  1. La página carga y pinta la lista con los contenedores conocidos por el último refresco.
  2. Cada fila muestra indicador de estado, nombre, imagen, distintivo de estado y, si corre, CPU y memoria.
  3. El bloque de lista se auto-refresca cada 10 s por sondeo, reemplazándose a sí mismo.
- **Salida:** el operador confirma de un vistazo que sus cinco contenedores están arriba.
- **Camino de error:** si el refresco no puede leer datos, se entra en el Flujo C sin recargar la página.

### Flujo B — Inspeccionar un contenedor y sus logs

- **Entrada:** el operador toca una fila de contenedor.
- **Pasos:**
  1. Se despliega el detalle con puertos, volúmenes y **nombres** de variables de entorno.
  2. El operador pulsa "View Logs (last 100 lines)" y el visor carga bajo demanda.
- **Salida:** diagnostica sin abrir SSH.
- **Camino de error:** si el detalle o los logs fallan, el bloque correspondiente muestra el
  componente de error en línea con su acción de reintento, y el resto del detalle permanece.

### Flujo C — El helper no responde

- **Entrada:** el operador abre `/docker`, o un refresco automático falla, con el helper caído,
  con el socket ausente o tras agotarse el timeout de 2-3 s.
- **Pasos:**
  1. La lista se sustituye por el componente de error en línea.
  2. El texto nombra la causa en términos del sistema, no del usuario: la fuente de datos no
     está disponible, no "no hay contenedores".
  3. El sondeo de 10 s sigue vivo, así que la recuperación es automática y no exige recargar.
- **Salida:** el operador entiende que el fallo es de lectura y que el resto del panel sigue fiable.
- **Camino de error:** ninguno más profundo — este ya es el estado de error.

### Flujo D — No hay contenedores, pero sí hay lectura

- **Entrada:** el helper responde correctamente con una lista de longitud cero.
- **Pasos:** se muestra el estado vacío estándar, con su icono y su texto de lista vacía.
- **Salida:** el operador sabe que Docker responde y que realmente no hay contenedores.
- **Diferencia clave con el Flujo C:** este estado solo es alcanzable con lectura confirmada.

### Flujo E — Intentar controlar un contenedor (flujo retirado)

Este flujo **deja de existir**. Antes: el operador pulsaba Start, Stop o Restart en la fila y
confirmaba en un modal. Ahora la zona de acción de la fila desaparece por completo — no queda
un botón deshabilitado ni un mensaje de "no permitido", porque un control visible pero inerte
invita a re-cablearlo y es justo lo que la frontera de seguridad prohíbe.

Los controles de **servicios systemd** en `/services` no se tocan y siguen su flujo actual.

---

## Component Inventory

### Pantalla: Docker (`/docker`)

| Componente | Estados | Comportamiento | Heurísticas de Nielsen |
|---|---|---|---|
| Encabezado de página | default | Texto descriptivo de la pantalla. Pierde la palabra "controls" de su subtítulo, que ya no describe la pantalla. | H2 correspondencia con el mundo real |
| Distintivo de indisponibilidad | visible / ausente | Se retira. Su función la absorbe el Error State, que es central y no periférico. Mantener ambos fue la causa del mensaje contradictorio. | H1 visibilidad del estado, H8 diseño minimalista |
| Contenedor de lista | idle / refrescando | Bloque que se auto-reemplaza cada 10 s por sondeo. Sin cambios. | H1 visibilidad del estado |
| Fila de contenedor | default / running / paused / stopped | Indicador de estado, nombre, imagen, distintivo de estado y, solo si corre, CPU y memoria. **Pierde su zona de acción.** | H4 consistencia, H6 reconocer antes que recordar |
| Zona de acción de fila | **eliminado** | Los tres botones y el modal de confirmación asociado desaparecen del árbol. | H8 diseño minimalista |
| Empty State | default | Reutilizado tal cual del inventario raíz. **Solo se renderiza con lectura confirmada y cero contenedores.** | H1 visibilidad del estado |
| Error State | default | Reutilizado tal cual del inventario raíz, con su acción de reintento. **Sustituye a la lista entera** cuando no hay lectura. | H1 visibilidad del estado, H9 ayudar a reconocer y recuperarse de errores |
| Detalle de contenedor | default / vacío / error | Puertos, volúmenes y nombres de variables de entorno. Nunca valores. | H10 ayuda y documentación |
| Log Drawer | idle / loading / success / error | Reutilizado del inventario raíz. Carga bajo demanda los últimos 100 renglones. | H1 visibilidad del estado |
| Confirmation Modal | **no usado en esta pantalla** | Sigue existiendo en el inventario raíz para los servicios systemd; deja de invocarse desde contenedores. | H5 prevención de errores |

**Reutilización, no invención:** Empty State, Error State y Log Drawer ya están en el
inventario de componentes de la raíz. Esta feature no añade ninguno. La única desviación
respecto del estándar del padre es la **retirada** de los botones de acción del componente
"Service Row" cuando se usa para un contenedor Docker; queda justificada abajo.

### Justificación de la desviación del estándar del padre

El inventario raíz define "Service Row" como *"Docker or Systemd row with status badge and
action buttons"*. Esta feature retira los botones de acción **solo en su uso para contenedores
Docker**, y los conserva intactos en su uso para servicios systemd.

Motivo: el hallazgo C2 de la auditoría del 2026-09-05. Que la web app pueda actuar sobre
Docker es exactamente la capacidad que convertía un fallo en el proceso web, expuesto en LAN
y tailnet, en control de root sobre la Pi. La frontera acordada es que el helper exponga
Docker en solo lectura, así que ningún control de contenedor puede existir en la interfaz.
La retirada es la representación visual de esa frontera, no una decisión estética.

Consecuencia registrada: el criterio AC-008-002 del FR-008 raíz queda obsoleto. Está anotado
como **BL-043** y se enmienda en un ciclo aparte, por decisión del owner del 2026-09-06.

---

## Nielsen Compliance

### Pantalla: Docker (`/docker`)

| Heurística | Cómo la satisface el diseño | Compromiso asumido |
|---|---|---|
| H1 Visibilidad del estado del sistema | Tres estados mutuamente excluyentes y nunca simultáneos: poblado, vacío con lectura confirmada, y sin lectura. El sondeo de 10 s hace visible la recuperación sin intervención. | El estado de error no distingue "helper caído" de "helper lento": ambos dicen que la fuente no está disponible. Distinguirlos exigiría exponer detalle interno al navegador sin beneficio para el operador. |
| H2 Correspondencia con el mundo real | El texto de error habla de no poder leer el estado de los contenedores, no de códigos ni de nombres de sockets. | Ninguno. |
| H3 Control y libertad del usuario | El Error State ofrece reintento manual además del sondeo automático. | El operador pierde la capacidad de arrancar y parar contenedores desde el panel. Es una pérdida deliberada de control, impuesta por la frontera de seguridad, y por eso se documenta en la ayuda en vez de esconderse. |
| H4 Consistencia y estándares | Empty State y Error State son los mismos componentes que usan el resto de listas del panel. Los tokens de color son los de la raíz. | Ninguno. |
| H5 Prevención de errores | Al no existir controles destructivos sobre contenedores, desaparece la clase entera de error de "paré el contenedor equivocado". | Ninguno. |
| H6 Reconocer antes que recordar | El estado de cada contenedor se lee del indicador y del distintivo sin recordar convenciones. | Ninguno. |
| H7 Flexibilidad y eficiencia | El detalle y los logs siguen bajo demanda, sin coste para quien solo mira la lista. | Ninguno. |
| H8 Diseño estético y minimalista | Se retiran una zona de acción por fila y un distintivo redundante. La pantalla queda con menos elementos y sin mensajes contradictorios. | Ninguno. |
| H9 Reconocer, diagnosticar y recuperarse | El error es central, nombra la causa y ofrece reintento. Ya no compite con un mensaje falso de lista vacía. | Ninguno. |
| H10 Ayuda y documentación | El detalle muestra los nombres de las variables de entorno, que es lo que permite diagnosticar sin exponer secretos. | Los valores no son consultables desde el panel, por diseño. |

---

## Design Tokens

**Esta feature no define ningún token nuevo.** Todos los que usa se heredan del estándar
del proyecto padre, que tiene prioridad sobre cualquier valor por defecto de arquetipo.
Se listan aquí con su rol y su razón porque el briefing exige que la sección exista y porque
el desarrollador implementa contra esta tabla.

| Rol | Token del padre | Valor | Razón de uso en esta feature |
|---|---|---|---|
| Fondo de página | `--color-base` | `#0b0c0f` | Fondo de la pantalla Docker, sin cambios. |
| Superficie de tarjeta | `--color-card` | `#1a1d23` | Superficie de cada fila de contenedor y del bloque de detalle. |
| Superficie elevada | `--color-surface` | `#121418` | Fondo del distintivo de estado en contenedores no activos. |
| Texto principal | `--color-text` | `#e5e7eb` | Nombre del contenedor y valores del detalle. |
| Texto secundario | `--color-text-muted` | `#9ca3af` | Imagen, métricas, etiquetas del detalle y texto del estado vacío. |
| Borde | `--color-border` | `#2a2f37` | Borde de fila y separador dentro del detalle. |
| Acento | `--color-accent` | `#c2c7d0` | Acción de abrir el visor de logs. |
| Error, fondo | `--color-error-bg` | `#4a1525` | Fondo del Error State que sustituye a la lista cuando no hay lectura. |
| Error, texto | `--color-error-text` | `#ff6b6b` | Texto del Error State. Es el token de error del panel; no se introduce un rojo paralelo. |
| Peligro | `--color-danger` | `#e34b6a` | Ya **no** se usa en esta pantalla: era el color del control de parada, que se retira. Se documenta para que su desaparición del árbol sea intencional y no parezca un olvido. |
| Estado correcto | verde de estado del padre | — | Indicador y distintivo de contenedor en marcha. Sin cambios. |
| Estado en pausa | amarillo de estado del padre | — | Indicador y distintivo de contenedor en pausa. Sin cambios. |
| Estado inactivo | `--color-text-muted` | `#9ca3af` | Indicador de contenedor detenido. Sin cambios. |

Tipografía, escala de espaciado, radios, movimiento e iconos: **sin desviación** respecto del
estándar de la raíz. Familia de sistema para texto y monoespaciada para métricas, rutas y logs,
tal como ya hace la pantalla.

### Requisitos medibles que esta pantalla debe cumplir

- Contraste del texto del Error State sobre su fondo ≥ 4.5:1, verificado sobre los tokens
  `--color-error-text` y `--color-error-bg` (NFR-100).
- El Error State y el Empty State **nunca** se renderizan a la vez (AC-091-002).
- Con el helper detenido, la pantalla responde 200 y muestra el Error State (AC-091-001).
- Con el helper respondiendo, la pantalla no contiene el texto del Error State (AC-091-003).
- La pantalla no contiene ninguna petición HTMX hacia una ruta de acción de contenedor (AC-094-001).
- La pantalla de servicios conserva sus controles (AC-094-003).
- Objetivos táctiles ≥ 44×44 px en viewports ≤ 768 px, heredado del estándar del padre, aplicable
  a la fila expandible y a la acción de logs.
- Movimiento ≤ 200 ms y `prefers-reduced-motion` respetado, heredado sin cambios.

Preview: not generated — la feature no introduce ningún token, componente ni composición nueva.
Su superficie visual es un cambio de lógica de estados sobre componentes ya aprobados en la raíz
(Empty State, Error State, Log Drawer) más la retirada de una zona de acción. Un preview solo
volvería a pintar el sistema de diseño existente, y el briefing prohíbe que un preview extienda
la especificación. La verificación real de esta pantalla son los criterios medibles de arriba,
que sí se ejecutan como tests.
