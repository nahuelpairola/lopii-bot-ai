# Dashboard de Grafana — admin

`admin-dashboard.json` es el dashboard de estado de la app. **Schema V2**
(`elements` + `layout`), 28 paneles: 9 visibles y 4 filas colapsadas de
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

## La cadena de modelos — por qué sale de `llm_calls` sin columna nueva

Cuando el modelo principal rebota por cupo, el loop prueba el siguiente
(`orchestrator.agentRound`). No hizo falta agregar nada para verlo: **cada
intento escribe su propia fila** en `llm_calls` —`c.record` corre también en el
camino de error— con su `model`, su `http_status` y el `trace_id` del turno. La
cadena se reconstruye ordenando las filas de un `trace_id` por fecha.

Tres paneles en "LLM y tokens":

- **Turnos perdidos por cupo** (stat) — el número titular: turnos que rebotaron y
  que ningún escalón salvó. Es el trabajo del fallback en un solo número, y va a
  cero. Conteo absoluto y no tasa, por la misma razón que los semáforos de arriba.
- **Cadena del agente** (tabla) — un escalón por fila. La columna **"entró de
  respaldo"** cuenta las veces que ese modelo atendió justo después de que otro
  rebotara: **reconstruye el orden de la cadena desde los datos, sin leer la
  config**. Verificado contra la base local el 2026-08-13, que sí tiene la cadena
  corrida: `gpt-oss-20b` 0 (es el principal), `gpt-oss-120b` 19, `llama-3.3-70b` 4
  sobre 4 intentos — y el último escalón nunca rebota.
- **Turnos del agente: quién los atendió** (barras apiladas) — un turno por
  `trace_id`, en tres bandas.

**Una columna mal rotulada acá es peor que una faltante.** El primer intento de la
columna de escalón usaba `row_number()` sobre el `trace_id`, y daba 1.2 para un
modelo que es SIEMPRE el principal: el contador cruzaba las rondas del loop, que
son varias por turno. Se descartó por eso, no por costo.

En el de las barras, "salvados por el respaldo" exige que el 200 venga **justo después
de un 429 y con OTRO modelo**. Sin esa condición el panel miente, y no de forma
sutil: un replay de la cola reusa el `trace_id`, así que un turno que rebotó y
se resolvió 40 minutos después contaba como rescate. Medido el 2026-08-13, la
versión ingenua marcaba 14 rescates sobre datos donde el fallback **ni siquiera
estaba deployado**. El rescate es instantáneo; el replay hizo esperar al
usuario. Son cosas distintas y la banda naranja ("a la cola") es la que el
fallback tiene que achicar.

**Línea de base antes del fallback** (prod, 30 días al 2026-08-13, para comparar
después de deployarlo): 137 turnos del agente, 63 con rebote, y **49 muertos —
el 35,8%**. El día peor, 23 turnos a la cola.

Y el después, ya medido en la base **local**, que es la única con la cadena
corrida: en el último bucket, **17 turnos salvados por el respaldo contra 1 a la
cola**; el día anterior, 0 y 14. Eso es lo que estos paneles tienen que mostrar en
prod una vez deployado.

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
- [ ] Expandir las cuatro filas colapsadas: sus 18 paneles también renderizan.
- [ ] En "LLM y tokens", los TRES paneles de la cadena. Ojo con
      **"Turnos del agente"**: es el único query del dashboard que le pasa a
      `$__timeGroupAlias` una columna de una subconsulta (`inicio`) en vez de
      una columna de tabla. El linter no puede ver si la macro expande bien ahí.
      "No data" es aceptable —hasta deployar el fallback la banda azul es cero
      por definición—; un error rojo de query no lo es.
- [ ] En "Cotizaciones e IPC": los tres semáforos en verde (días ≤ 4, meses ≤ 2,
      hueco ≤ 4) y el MEP dibujando línea continua. Confirmar que el primer y el
      último punto del MEP caen en el día correcto — es el bug de huso, y con
      `timezone: browser` sólo se ve desde un navegador en ART.
- [ ] Copiar un `trace_id` de la tabla de errores y confirmar que aparece en los
      logs de Render. Si no hay errores en el rango, ampliar el time picker o
      validar contra `request_traces` directo.
