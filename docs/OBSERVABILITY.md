# Observability guide (Grafana, Prometheus, Loki, Tempo)

This guide explains how to view metrics, logs, and traces for this app locally, and how
to build your own Grafana dashboards on top of them. It also collects verification and
troubleshooting commands for the whole observability stack.

The stack is defined in [`docker-compose.yml`](../docker-compose.yml) and configured under
[`observability/`](../observability/). It is optional and only affects local development.

## What's running and why

| Service | Purpose | Host port |
|---|---|---|
| `otel-collector` | Receives OTLP metrics/traces, exposes them for Prometheus, forwards traces to Tempo | 4317 (gRPC), 4318 (HTTP) |
| `prometheus` | Scrapes/stores metrics from `otel-collector` and `postgres-exporter` | 9090 |
| `postgres-exporter` | Exposes PostgreSQL metrics (`pg_*`) for Prometheus to scrape | 9187 |
| `tempo` | Stores and serves traces | 3200 |
| `loki` | Stores and serves logs | 3100 |
| `promtail` | Discovers all compose containers via the Docker socket and ships their stdout/stderr logs to Loki | — |
| `grafana` | UI for querying Prometheus/Loki/Tempo and viewing dashboards | 3000 |

The app itself is instrumented end-to-end. Today:

- **Logs**: real — `promtail` tails every container's Docker logs (including `app`) and ships
  them to Loki. You can see the app's actual log lines in Grafana right now.
- **Metrics**: real for both PostgreSQL (via `postgres-exporter`) and the app itself, which
  exposes native Prometheus metrics at `GET /metrics` (`app_http_requests_total`,
  `app_http_request_duration_seconds`).
- **Traces**: real — the app uses the OpenTelemetry Go SDK to export a span per HTTP request
  via OTLP/gRPC to `otel-collector`, which forwards them to Tempo. Trace/span IDs are also
  included in the app's structured logs for Loki\u2194Tempo correlation.

The `telemetrygen-traces`/`telemetrygen-metrics` services (behind the `synthetic` profile)
are still available if you want extra synthetic sample data unrelated to real app traffic.

## Starting the stack

```bash
cd /Users/edino.moniz/Development/onidemon37/tasting-journals-app
docker compose up -d --build
```

Only one Compose stack can run at a time on these ports. If you also work in
`jira-org-sync`, stop its stack first: `(cd ../jira-org-sync && docker compose down)`.

Optional: also run the synthetic telemetry generators (fake traces/metrics, useful for
practicing dashboard building before the app emits real telemetry):

```bash
docker compose --profile synthetic up -d telemetrygen-traces telemetrygen-metrics
```

## Viewing logs in Grafana (works today)

1. Open http://localhost:3000 (no login needed, anonymous admin is enabled).
2. Left nav → **Explore** (compass icon).
3. Top-left datasource dropdown → select **Loki**.
4. Enter a query and run it (Shift+Enter or the blue "Run query" button):
   - All app logs: `{compose_service="app"}`
   - Everything in this project: `{compose_project="tasting-journals-app"}`
   - Only errors: `{compose_service="app"} |= "error"`
5. There's also a pre-built dashboard: **Dashboards → Logs (Loki)**.

Available Loki labels: `compose_project`, `compose_service`, `container`, `service_name`.

### Searching logs (LogQL)

LogQL queries start with a label selector, then optionally pipe through filters/parsers:

- Label selector only, all lines: `{compose_service="app"}`
- Plain-text substring filter: `{compose_service="app"} |= "error"` (`!=` to exclude)
- Parse the JSON log line into fields: `{compose_service="app"} | json`
- Filter on a parsed field: `{compose_service="app"} | json | status >= 500`
- Filter on request path: `{compose_service="app"} | json | path="/tastings"`
- Reformat the displayed line: `{compose_service="app"} | json | line_format "{{.method}} {{.path}} -> {{.status}} ({{.duration_ms}}ms)"`
- Find the log line for a specific trace (see Tempo section below for how to get a `trace_id`): `{compose_service="app"} | json | trace_id="<trace_id>"`

## Viewing PostgreSQL metrics in Grafana (works today)

1. **Dashboards → PostgreSQL Overview** — pre-built panels for connections, transaction
   rate, cache hit ratio, and rows read/written.
2. Or explore ad hoc: **Explore** → datasource **Prometheus** → try:
   - `pg_up`
   - `pg_stat_database_numbackends{datname="tasting_journals"}`
   - `rate(pg_stat_database_xact_commit{datname="tasting_journals"}[5m])`

## Viewing app metrics in Grafana (works today)

The app exposes native Prometheus metrics at `GET /metrics` (job `app` in
[`prometheus.yml`](../observability/prometheus.yml)):

- `app_http_requests_total{method, route, status}` — request counter.
- `app_http_request_duration_seconds{method, route}` — request duration histogram.

Explore ad hoc: **Explore** → datasource **Prometheus** → try:

- `sum by (route) (rate(app_http_requests_total[5m]))` — request rate per route.
- `sum by (status) (rate(app_http_requests_total[5m]))` — request rate per status code.
- `histogram_quantile(0.95, sum by (le, route) (rate(app_http_request_duration_seconds_bucket[5m])))` — p95 latency per route.

## Viewing traces in Grafana (works today)

The app is instrumented with the OpenTelemetry Go SDK (see
[`cmd/server/telemetry.go`](../cmd/server/telemetry.go)) and exports real spans for every
HTTP request via OTLP/gRPC to the collector, which forwards them to Tempo.

1. **Dashboards → Traces (Tempo)**, or **Explore** → datasource **Tempo** → TraceQL search.
2. Search by service: `{ .service.name = "tasting-journals-app" }` (matches the
   `OTEL_SERVICE_NAME` set on the `app` service in [`docker-compose.yml`](../docker-compose.yml)).
3. Narrow by route: `{ .service.name = "tasting-journals-app" && name = "GET /tastings" }`.
4. Narrow by status: `{ .service.name = "tasting-journals-app" && status = error }`.
5. Click into a trace to see its spans, or copy its trace ID to cross-reference in Loki
   (every app log line includes `trace_id`/`span_id` fields — see the LogQL section above).

Tracing is optional and controlled entirely by env vars on the `app` service: it's a no-op
(no network calls, no overhead) unless `OTEL_EXPORTER_OTLP_ENDPOINT` is set, which it is by
default in [`docker-compose.yml`](../docker-compose.yml).

### Searching traces (TraceQL)

TraceQL filters on span/resource attributes, similar to PromQL/LogQL:

| Query | What it finds |
|---|---|
| `{ .service.name = "tasting-journals-app" }` | everything from this service |
| `{ name = "GET /tastings" }` | only that route (span name = route, via `mux.Handler(r)`) |
| `{ status = error }` | only failed requests (5xx sets span status to Error) |
| `{ .http.status_code = 500 }` | filter on the specific attribute set by the middleware |
| `{ .http.method = "POST" }` | only POST requests |
| `{ duration > 100ms }` | slow requests — good for finding latency outliers |
| `{ .service.name = "tasting-journals-app" && duration > 50ms }` | combine conditions with `&&` |

Currently each trace has a single span (the HTTP request as a whole) — see "Adding child
spans" below to break a trace down into sub-operations (e.g. DB query vs. handler logic).

### Correlating traces with logs and metrics

- **Trace → logs**: copy the `traceID` from a Tempo trace, then in Loki:
  `{compose_service="app"} | json | trace_id="<traceID>"`.
- **Log → trace**: find an interesting log line in Loki (e.g. a 500), grab its `trace_id`
  field, then search Tempo by trace ID directly (Explore → Tempo → "Trace ID" search mode,
  or `{ trace:id = "<traceID>" }`).
- **Metrics → traces**: if a Prometheus query shows a latency spike or error rate increase
  for a route (e.g. `histogram_quantile(0.95, sum by (route) (rate(app_http_request_duration_seconds_bucket[5m])))`),
  jump to Tempo and filter `{ name = "<that route>" && duration > <threshold> }` to find the
  actual slow/failed traces behind the aggregate number.

### Adding child spans (going deeper)

Right now every trace is one flat span per request — useful for "was this request
slow/did it error," but not for "which part was slow." To break a trace down into
sub-operations, start a child span around specific work inside a handler, e.g.:

```go
ctx, span := tracer.Start(ctx, "db.query.list_tastings")
defer span.End()
rows, err := pool.Query(ctx, "...")
```

As long as the same `ctx` (derived from the request's context) is passed through, the SDK
automatically nests the child span under the request span, and the trace view in Grafana
shows a waterfall breakdown (HTTP span → DB span → ...) with each segment's duration.

## Building your own dashboard

Dashboards are provisioned from JSON files in [`observability/dashboards/json/`](../observability/dashboards/json/)
via [`observability/dashboards/dashboards.yaml`](../observability/dashboards/dashboards.yaml). You have two options:

### Option A — build in the UI, then export (recommended for exploring)

1. In Grafana, **Dashboards → New → New Dashboard → Add visualization**.
2. Pick a datasource (**Prometheus**, **Loki**, or **Tempo**) and write a query as above.
3. Repeat "Add visualization" for more panels, arrange/resize them, then **Save dashboard**.
4. To persist it as code: dashboard **Settings (gear icon) → JSON Model**, copy the JSON,
   save it under `observability/dashboards/json/<name>.json`, and give it a stable
   `"uid"`. It will be picked up automatically (the provisioner polls every 30s) — no
   restart needed. Reference datasources by name string (e.g. `"datasource": "Prometheus"`),
   not by UID, so they resolve correctly on any machine.

### Option B — copy an existing dashboard JSON as a starting point

Use one of the existing files as a template:

- [`postgres-overview.json`](../observability/dashboards/json/postgres-overview.json) — Prometheus timeseries/stat panels.
- [`loki-logs.json`](../observability/dashboards/json/loki-logs.json) — a Loki logs panel.
- [`tempo-traces.json`](../observability/dashboards/json/tempo-traces.json) — a Tempo traces panel.

Copy one, change `"uid"`, `"title"`, and the panel queries, drop it in
`observability/dashboards/json/`, and it appears in Grafana within ~30s.

## Verifying the stack end-to-end

Run these from the repo root after `docker compose up -d --build`.

### Container status

```bash
docker compose ps -a --format "table {{.Name}}\t{{.Status}}\t{{.Ports}}"
```

All services should show `Up` (postgres additionally `(healthy)`). If a service shows no
published port under `Ports` even though the compose file defines one, it may be a stale
container from a previous failed start — force-recreate it:

```bash
docker compose up -d --force-recreate <service>
```

### Prometheus scrape targets

```bash
curl -s 'http://localhost:9090/api/v1/targets' \
  | python3 -c "import json,sys; d=json.load(sys.stdin); [print(t['labels']['job'], t['health']) for t in d['data']['activeTargets']]"
```

Expected: `otel-collector up`, `postgres-exporter up`, and `app up`.

### PostgreSQL metrics flowing

```bash
curl -s 'http://localhost:9090/api/v1/query?query=pg_up' | python3 -m json.tool
```

Expected: a result with value `1`.

### App metrics flowing

```bash
curl -s 'http://localhost:9090/api/v1/query?query=app_http_requests_total' | python3 -m json.tool
```

Expected: at least one series with `job="app"` (generate traffic first if empty:
`curl http://localhost:8080/`).

### Loki receiving logs

```bash
curl -s 'http://localhost:3100/loki/api/v1/labels'
curl -s 'http://localhost:3100/loki/api/v1/label/compose_project/values'
curl -s 'http://localhost:3100/loki/api/v1/query?query=%7Bcompose_project%3D%22tasting-journals-app%22%7D&limit=3' | python3 -m json.tool
```

Expected: labels include `compose_project`, `compose_service`, `container`, `service_name`,
and the query returns recent log lines from the `app` container (or others).

### Tempo reachable

```bash
curl -s -o /dev/null -w "tempo:%{http_code}\n" http://localhost:3200/status
```

Expected: `tempo:200`. If it's `000`, see "Stale/unbound containers" below.

### App traces flowing

```bash
curl -s 'http://localhost:3200/api/search?tags=service.name%3Dtasting-journals-app&limit=5' \
  | python3 -c "import json,sys; d=json.load(sys.stdin); [print(t['rootTraceName']) for t in d['traces']]"
```

Expected: a list of recent route names (e.g. `GET /tastings`, `GET /healthz`). If empty,
generate traffic first (`curl http://localhost:8080/`) and re-run after a few seconds
(spans are batched before export).

### Grafana healthy and dashboards provisioned

```bash
curl -s http://localhost:3000/api/health
curl -s 'http://localhost:3000/api/search?type=dash-db' | python3 -m json.tool
```

Expected: `"database": "ok"`, and 3 dashboards listed (`PostgreSQL Overview`,
`Logs (Loki)`, `Traces (Tempo)`).

## Troubleshooting

### Port conflicts with another project (e.g. `jira-org-sync`)

Both projects default to the same host ports (3000, 3100, 3200, 4317-4318, 9090). Only run
one stack at a time, or stop the other:

```bash
(cd ../jira-org-sync && docker compose down)
```

Symptom: `Bind for 0.0.0.0:3200 failed: port is already allocated` during `docker compose up`.

### A service is `Up` but has no published port / curl returns exit code 000 / connection refused

This can happen if a container was created during a failed `docker compose up` (e.g. one
that aborted partway due to a port conflict) and never got its network port bindings set
up, even though it later shows as "Up". Force-recreate it:

```bash
docker compose up -d --force-recreate tempo prometheus loki otel-collector grafana promtail
docker ps --filter "name=tasting-journals-app" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
```

Confirm the `Ports` column now shows `0.0.0.0:<port>->...` for each service before retrying.

### Tempo crash-looping with `field compactor not found in type app.Config`

`grafana/tempo:latest` moved to a new architecture incompatible with the classic
`compactor:` config. This repo pins `tempo` to `grafana/tempo:2.7.2` in
[`docker-compose.yml`](../docker-compose.yml), which is the fix — if you see this error, check
that the pin hasn't been reverted to `:latest`.

### "I only see data for PostgreSQL, nothing for the app"

The app now emits real logs, metrics, and traces:

| | Logs | Metrics | Traces |
|---|---|---|---|
| `app` | ✅ real (via Promtail) | ✅ real (`/metrics`, scraped by Prometheus) | ✅ real (OTel SDK → collector → Tempo) |
| PostgreSQL | — | ✅ real (via `postgres-exporter`) | — |

If you're not seeing app data:

1. Grafana → **Explore** → datasource **Loki**, query `{compose_service="app"}`
2. Datasource **Prometheus**, query `app_http_requests_total`
3. Datasource **Tempo**, TraceQL `{ .service.name = "tasting-journals-app" }`
4. Make sure the time range (top-right) covers when you used the app, e.g. "Last 1 hour"
5. If all three are empty, generate some traffic first (`curl http://localhost:8080/`) and
   re-run — traces in particular are batched and may take a few seconds to appear.

The synthetic generators (`docker compose --profile synthetic up -d telemetrygen-traces
telemetrygen-metrics`) are still available if you want extra sample data unrelated to the
real app traffic.

### Dashboard shows "No data"

- Confirm you're looking at the right time range (top-right time picker) — default is
  often "Last 6 hours", which is fine, but a custom narrow range can hide recent data.
- Re-run the panel's query directly in **Explore** to confirm the underlying data exists
  (see verification commands above).
- For the Loki dashboard specifically, confirm `compose_project` matches your directory
  name: `curl -s 'http://localhost:3100/loki/api/v1/label/compose_project/values'`.

### Dashboard JSON changes aren't showing up

The file provisioner polls every 30 seconds (`updateIntervalSeconds: 30` in
[`dashboards.yaml`](../observability/dashboards/dashboards.yaml)). Wait, or restart Grafana:

```bash
docker compose restart grafana
```
