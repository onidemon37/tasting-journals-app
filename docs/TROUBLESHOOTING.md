# Troubleshooting with curl

These commands assume the local Docker Compose application is available at:

```text
http://127.0.0.1:8080
```

Set a reusable URL:

```bash
BASE_URL=http://127.0.0.1:8080
```

Do not include passwords, database URLs, API tokens, cookies, or authorization headers in commands that may be copied into shell history or CI logs.

## Check the containers

```bash
docker compose ps
docker compose logs --tail=100 app
docker compose logs --tail=100 postgres
```

The app should be running and PostgreSQL should be healthy before testing HTTP endpoints.

## Check process health

The liveness endpoint should return HTTP 200 without requiring PostgreSQL:

```bash
curl --fail --show-error --silent \
  --write-out '\nHTTP %{http_code}\n' \
  "$BASE_URL/healthz"
```

Expected response:

```text
{"status":"ok"}
HTTP 200
```

A failure usually means the container is not listening, the port is wrong, or the application process exited.

## Check database readiness

The readiness endpoint checks the PostgreSQL connection:

```bash
curl --show-error --silent \
  --write-out '\nHTTP %{http_code}\n' \
  "$BASE_URL/readyz"
```

Expected response:

```text
{"status":"ready"}
HTTP 200
```

HTTP 503 means the application is running but cannot reach PostgreSQL. Check:

```bash
docker compose ps postgres
docker compose logs --tail=100 postgres
docker compose logs --tail=100 app
```

## Inspect response headers

Use `-i` to inspect status and headers while keeping the response body visible:

```bash
curl --include --silent --show-error "$BASE_URL/healthz"
```

The response should include:

```text
Content-Type: application/json
X-Request-ID: <request-id>
```

## Trace a request

Send your own request ID, then search for it in the application logs:

```bash
REQUEST_ID="troubleshoot-$(date +%s)"
curl --fail --silent --show-error \
  -H "X-Request-ID: $REQUEST_ID" \
  "$BASE_URL/tastings" >/dev/null

docker compose logs app | grep "$REQUEST_ID"
```

The log record should be JSON and contain the request method, path, status, duration, peer IP, remote address, user agent, and request ID.

## Test route status codes

Check a valid route:

```bash
curl --silent --show-error --write-out 'HTTP %{http_code}\n' --output /dev/null \
  "$BASE_URL/tastings"
```

Check a missing tasting:

```bash
curl --silent --show-error --write-out 'HTTP %{http_code}\n' --output /dev/null \
  "$BASE_URL/tastings/999999"
```

A missing record should return HTTP 404. If it returns 500, inspect the app logs and database connection.

## List and search tastings

List all records:

```bash
curl --fail --silent --show-error "$BASE_URL/api/tastings"
```

Search by name, distillery, region, overall notes, or tag:

```bash
curl --fail --silent --show-error \
  --get "$BASE_URL/api/tastings" \
  --data-urlencode 'q=speyside'
```

`--data-urlencode` safely handles spaces and special characters in search terms.

## Create a test tasting

Use a clearly named local test record:

```bash
curl --fail --silent --show-error \
  -X POST "$BASE_URL/api/tastings" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Troubleshooting Dram",
    "distillery": "Local Test Distillery",
    "region": "Test Region",
    "country": "Scotland",
    "age": 12,
    "abv": 46,
    "caskType": "Ex-bourbon",
    "rating": 80,
    "tags": ["local-test"],
    "nose": "Test nose",
    "palate": "Test palate",
    "finish": "Test finish",
    "overall": "Test record",
    "whatLearned": "This record is for local troubleshooting."
  }'
```

The response should return HTTP 201 and include the generated `id`.

## Update a test tasting

Replace `ID` with the generated ID:

```bash
ID=1
curl --fail --silent --show-error \
  -X PUT "$BASE_URL/api/tastings/$ID" \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Updated Troubleshooting Dram",
    "rating": 85,
    "overall": "Updated local test record"
  }'
```

## Delete a test tasting

Only delete records created for local testing:

```bash
curl --fail --silent --show-error \
  -X DELETE \
  --write-out '\nHTTP %{http_code}\n' \
  "$BASE_URL/api/tastings/$ID"
```

A successful delete returns HTTP 204.

## Validate malformed input

The API should reject an empty name:

```bash
curl --silent --show-error \
  -X POST "$BASE_URL/api/tastings" \
  -H 'Content-Type: application/json' \
  -d '{"rating": 90}' \
  --write-out '\nHTTP %{http_code}\n'
```

It should return HTTP 400. Ratings outside `0` to `100` and ABV values outside `0` to `100` should also be rejected.

## Test forwarded IP logging

This tests diagnostic header logging only. Forwarded headers are untrusted unless a known proxy adds them:

```bash
REQUEST_ID="proxy-test-$(date +%s)"
curl --silent --show-error \
  -H "X-Request-ID: $REQUEST_ID" \
  -H 'X-Forwarded-For: 203.0.113.10' \
  -H 'X-Real-IP: 203.0.113.10' \
  "$BASE_URL/healthz" >/dev/null

docker compose logs app | grep "$REQUEST_ID"
```

The JSON log should include `peer_ip`, `remote_addr`, `forwarded_for`, and `real_ip_header`. The application must use the actual peer address for security decisions unless proxy trust is explicitly configured.

## Reset local data

Remove the local PostgreSQL volume and recreate the stack:

```bash
docker compose down -v
docker compose up --build
```

This destroys local tasting records. Never run it against a production database.
