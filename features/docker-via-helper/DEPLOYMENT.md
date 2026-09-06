# DEPLOYMENT — docker-via-helper (v1.1.0)

**Modelo de despliegue: binarios nativos.** Sin contenedores, sin imágenes, sin orquestador —
una ironía útil de recordar: el panel *observa* Docker, no se *despliega* en Docker. Por eso no
hay `Dockerfile` ni `docker-compose.yml` en esta feature, y no debe haberlos.

---

## Prerrequisitos

En la Pi, ya cumplidos desde la auditoría del 2026-09-05 (verificar, no re-hacer):

- **El usuario `ultron` NO pertenece al grupo `docker`.** Es la causa raíz del hallazgo C2 y no
  debe revertirse nunca. Comprobar con `id ultron`.
- `ultron-helper.service` corre como `User=root` y expone `/run/ultron-helper.sock` con permisos
  `srw-rw---- root:ultron`.
- `ultron-ap.service` corre como `User=ultron`.
- Docker escucha en `/var/run/docker.sock`, accesible solo por root.

**Sin cambios de configuración.** Esta feature no añade ninguna variable de entorno, no toca
`systemd` ni `sudoers`. Ver `.env.example` para las variables existentes que gobiernan el canal.

## Entorno de desarrollo

```sh
go test ./...                                  # suite completa
go test ./... -run TestTC_DVH -v               # solo los 59 TC de esta feature
sh scripts/gofmt-check.sh                      # formato de los paquetes de la feature
sh scripts/build-arm64-check.sh                # compilación cruzada al objetivo real
go vet ./...
```

No hace falta Docker en la máquina de desarrollo: cada test levanta un demonio simulado sobre
un socket Unix temporal.

## Construcción

```sh
make build-arm    # compila ultron-ap y ultron-helper para linux/arm64, con el commit en ldflags
```

En la Pi **no hay checkout del código fuente**. Todo se compila de forma cruzada en el Mac.

## Despliegue a producción

> **Los DOS binarios cambian en esta feature y deben desplegarse juntos.** Un despliegue parcial
> deja una combinación rota: el helper viejo no conoce las acciones `docker.list`, `docker.inspect`
> ni `docker.logs`, responde `unknown action`, y la pantalla queda en estado de error permanente.
> Es el riesgo R1 de `02_SYSTEM_DESIGN.md`. El modo degradado es explícito y diagnosticable —
> ese es el punto de FR-091 — pero sigue siendo un panel roto.

Desde la raíz del repo, en el Mac:

```sh
make build-arm
scp bin/ultron-ap-linux-arm64     cesareyeserrano@192.168.1.29:/tmp/ultron-ap
scp bin/ultron-helper-linux-arm64 cesareyeserrano@192.168.1.29:/tmp/ultron-helper
```

En la Pi, **una sola llamada `sudo`** para que la contraseña se pida una vez. Desde el
endurecimiento del 2026-09-06, `install`, `cp` y `chown` piden contraseña (solo `systemctl
start/stop` de estas dos unidades quedó sin ella), así que hace falta un TTY:

```sh
ssh -t cesareyeserrano@192.168.1.29 'sudo sh -c "
  systemctl stop ultron-ap ultron-helper &&
  cp /opt/ultron-ap/ultron-ap     /opt/ultron-ap/ultron-ap.prev &&
  cp /opt/ultron-ap/ultron-helper /opt/ultron-ap/ultron-helper.prev &&
  install -m 0755 /tmp/ultron-ap     /opt/ultron-ap/ultron-ap &&
  install -m 0755 /tmp/ultron-helper /opt/ultron-ap/ultron-helper &&
  chown root:root /opt/ultron-ap/ultron-helper &&
  systemctl start ultron-helper ultron-ap
"'
```

**No volver a añadir reglas `NOPASSWD` para `install` desde `/tmp`.** Un directorio escribible por
todos como origen de un binario que corre como root fue el hallazgo crítico C1.

## Verificación posterior

```sh
make deploy-verify        # compara los SHA de AMBOS binarios contra los locales
```

En la Pi:

```sh
id ultron                                     # NO debe incluir 'docker'
curl -s localhost:8080/version                # debe decir v1.1.0 y el commit desplegado
journalctl -u ultron-ap -n 50 --no-pager      # sin 'permission denied', sin 'daemon not reachable'
journalctl -u ultron-helper -n 50 --no-pager  # sin errores al atender docker.list
```

En el navegador, pestaña de contenedores: deben aparecer `homeassistant`, `hermes`, `openclaw`,
`ledger` y `mosquitto` con su estado y su consumo. **Ya no hay botones de arranque, parada ni
reinicio** — es lo esperado, no un fallo.

Un puerto cerrado justo después del reinicio es la ventana de reinicio, no una caída.

## Health checks

- `GET /health` → 200 mientras el proceso vive. Sin cambios; esta feature no añade endpoints.
- Vivacidad del helper: la acción `ping` de su protocolo, ya existente.
- **La caída del helper NO es una caída del servicio.** El resto del panel sigue en 200 y la
  sección de contenedores muestra su estado de error. No conviene alertar sobre `/health` por esto.

## Rollback

Los binarios anteriores quedaron como `.prev` en el paso de despliegue:

```sh
ssh -t cesareyeserrano@192.168.1.29 'sudo sh -c "
  systemctl stop ultron-ap ultron-helper &&
  cp /opt/ultron-ap/ultron-ap.prev     /opt/ultron-ap/ultron-ap &&
  cp /opt/ultron-ap/ultron-helper.prev /opt/ultron-ap/ultron-helper &&
  chown root:root /opt/ultron-ap/ultron-helper &&
  systemctl start ultron-helper ultron-ap
"'
```

**El rollback devuelve el panel al estado roto**, no al estado bueno: el binario anterior intenta
abrir `/var/run/docker.sock`, que el usuario `ultron` ya no puede alcanzar, así que la sección de
contenedores volverá a quedar vacía con el mensaje engañoso de "No containers found".

**Revertir el código NO justifica devolver `ultron` al grupo `docker`.** Eso reabriría el hallazgo
C2, que es mucho peor que una pestaña vacía. Si el rollback hace falta, se convive con la pestaña
rota hasta arreglar el binario.

## Nota sobre el gate de seguridad

Esta feature vacía `.github/vuln-allowlist.txt`. **Es obligatorio y va en el mismo commit**: el
gate falla tanto por una vulnerabilidad no listada como por una entrada listada que ya no se
reporta (protección contra allowlist podrida). Verificado localmente con `govulncheck` v1.7.0:
`Reachable: none`, y `GO-2026-4887` y `GO-2026-4883` no aparecen en el escaneo.
