# Dashboard de Grafana — admin

`admin-dashboard.json` es el dashboard de estado de la app. **Schema V2**
(`elements` + `layout`), 25 paneles: 9 visibles y 4 filas colapsadas de
drill-down.

> **Por qué V2 y no el schema clásico.** La instancia corre Grafana 13.2, que
> rechaza el schema v1 (`panels[]` + `gridPos`) al importar. En V2 los paneles
> viven en el mapa `elements` y su posición va aparte, en `layout`; el
> datasource se referencia por `name`, no por `uid`. Si alguna vez hay que
> volver a v1 para una instancia vieja, la ruta `POST /api/dashboards/db` de la
> API clásica todavía lo acepta — pero el import por UI de esta instancia, no.

Diseño y justificación de cada panel:
[`docs/superpowers/specs/2026-07-22-grafana-admin-dashboard-v2-design.md`](../superpowers/specs/2026-07-22-grafana-admin-dashboard-v2-design.md).

## Importar

1. Grafana → Dashboards → New → Import.
2. Pegar el contenido de `admin-dashboard.json`.
3. Elegir el datasource de Postgres cuando lo pida (variable `DS_POSTGRES`).

El archivo no trae ningún UID de datasource hardcodeado a propósito: era un id
específico del entorno de dev dentro de un archivo versionado.

## Cómo leerlo

**Fila 1 — ¿está roto?** Seis semáforos. Conteos absolutos y latencia máxima,
no tasas ni percentiles: con menos de 10 usuarios, "8.3% de error" es un error
sobre doce requests y mañana da 0% sin que haya cambiado nada.

**Fila 2 — ¿qué pasó?** Los últimos 50 errores de app y de LLM en una tabla.
El `trace_id` es la columna clave: `internal/logging` estampa ese mismo id en
toda línea de log del request, así que copiándolo a los logs de Render se
recupera el request entero.

**Fila 3 — ¿cómo viene?** Actividad (conteos) y latencia (ms) en paneles
separados, porque juntarlos exigiría un segundo eje Y.

**Filas colapsadas.** Producto (embudo intent × outcome, y la tasa de tap de
los tips), LLM y tokens, Higiene, Cotizaciones e IPC.

## Cotizaciones e IPC — por qué acá sí hay semáforos de frescura

Esta fila es la excepción a la regla de abajo, y por un motivo concreto: la
ingesta de `usd_quotes`/`monthly_cpi` **no depende de que un usuario escriba**.
La serie se actualiza todos los días hábiles pase lo que pase, así que
"hace 9 días que no entra una cotización" sí distingue roto de tranquilo,
que es justo lo que `max(received_at)` no puede hacer.

Los tres semáforos miden fallas distintas y ninguno tapa al otro:

- **Días sin cotización** — la ingesta entera parada. Verde hasta 4 porque la
  fuente publica con un día de atraso y el finde puede no cotizar.
- **Hueco más largo (90d)** — el único que ve un agujero *viejo*. Si el
  backfill se rompe pero el fetch de las 20:00 sigue andando, `max(date)`
  queda en hoy y "Días sin cotización" se queda verde con la serie agujereada.
- **Última cotización por tipo** — la falla *parcial*: que la fuente deje de
  publicar un `rate_type` mientras los otros seis llegan. Ningún agregado lo ve.

Los dos paneles de datos (MEP e IPC) están para lo que los semáforos no miran:
un valor absurdo. Una tabla de IPC en cero pasa "Meses sin IPC" en verde.

**Ojo con el huso.** El dashboard corre en `timezone: browser` y estas dos
tablas guardan `date`/`month`, no timestamps. Una `date` cruda se ancla a
medianoche UTC y para un lector en ART se dibuja el día anterior. Por eso el
panel del MEP ancla cada punto al **mediodía** y la tabla por tipo emite la
fecha con `to_char`, como texto. Cualquier panel nuevo sobre estas tablas tiene
que hacer lo mismo.

## Liveness NO está acá

Ningún panel dice si el bot está vivo. `max(received_at)` no distingue "bot
caído" de "nadie escribió en seis horas", que a esta escala pasa todas las
noches, y un panel que da rojo por diseño entrena a ignorar el rojo.

El liveness real está en el health check de Render contra `/health/internal`.
Para alerta de caída: un monitor externo contra `/health/external`.

## Modificarlo

`dashboard_test.go` corre dentro de `go test ./...` y valida:

- que el JSON parsee, tenga elementos y use `RowsLayout`;
- que ningún query combine `$__timeGroupAlias(...)` con un ` AS time` extra
  (**el macro ya emite su propio `AS "time"`** — esa doble alias rompió los 8
  paneles de serie temporal durante semanas sin que nada fallara);
- que todo `$__timeGroup(...)` sí lleve ` AS time`, porque ese no emite alias;
- que el detector de la doble alias siga reconociendo la query exacta que se
  rompió, para que un retoque futuro del regex no lo deje inútil;
- que todo query use el datasource `${DS_POSTGRES}` y no un UID literal;
- que `elements` y `layout` estén en correspondencia exacta: cada panel
  colocado una sola vez, ningún panel huérfano, ninguna referencia a un nombre
  inexistente. Es el modo de falla propio de V2 — un panel puede existir sin
  que nada lo ubique en pantalla, y eso no es un error de JSON, es un panel
  invisible;
- que la geometría no se salga de las 24 columnas.

Después de editar el JSON, correr `go test ./docs/grafana/` y **volver a
importar en Grafana**: el linter no puede ver si un panel renderiza. Ver la
checklist de abajo.

## Checklist de verificación manual

Obligatoria después de cualquier cambio. Un JSON que parsea no es un dashboard
que funciona: el archivo anterior parseaba perfecto y tenía 8 paneles rotos.

- [ ] Importa sin error.
- [ ] Los 9 paneles visibles renderizan. "No data" es aceptable (puede no haber
      filas en el rango); un error rojo de query no lo es.
- [ ] Los dos paneles de serie temporal dibujan línea con el rango `now-24h`.
      Esta es la regresión concreta que motivó la reescritura.
- [ ] Expandir las cuatro filas colapsadas: sus 15 paneles también renderizan.
- [ ] En "Cotizaciones e IPC": los tres semáforos en verde (días ≤ 4, meses ≤ 2,
      hueco ≤ 4) y el MEP dibujando línea continua. Confirmar que el primer y el
      último punto del MEP caen en el día correcto — es el bug de huso, y con
      `timezone: browser` sólo se ve desde un navegador en ART.
- [ ] Copiar un `trace_id` de la tabla de errores y confirmar que aparece en los
      logs de Render. Si no hay errores en el rango, ampliar el time picker o
      validar contra `request_traces` directo.
