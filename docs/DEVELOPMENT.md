# Development Guide

`tasting-journals-app` is a reusable Go and PostgreSQL application for recording whisky and other tasting journals. Sociedade 67 is one possible deployment brand; the application name is configured at runtime.

## Requirements

For application development:

- Go 1.24 or newer
- Docker and Docker Compose
- `kubectl` for Kubernetes manifest validation
- Minikube or Kind for local Kubernetes testing

Install the pinned Go toolchain with:

```bash
make install-go
```

This uses `mise` when available, then Homebrew. If neither is installed, the Makefile keeps Go checks available through Docker.

The host Go toolchain is optional when Docker is available. The Dockerfile can build and test the application using the Go builder image.

## Run with Docker Compose

Start the application and PostgreSQL together:

```bash
docker compose up --build
```

The application listens on:

```text
http://127.0.0.1:8080
```

Useful URLs:

- Journal: http://127.0.0.1:8080/tastings
- New tasting: http://127.0.0.1:8080/tastings/new
- Health: http://127.0.0.1:8080/healthz
- Readiness: http://127.0.0.1:8080/readyz

Stop the stack:

```bash
docker compose down
```

The Compose PostgreSQL volume persists local data. Remove it when a clean database is required:

```bash
docker compose down -v
```

The credentials in `docker-compose.yml` are development-only and must not be reused in production.

## Configuration

Copy `.env.example` for local reference. The main settings are:

```text
DATABASE_URL
APP_ADDRESS
APP_NAME
```

`APP_NAME` controls the runtime brand. For example:

```text
APP_NAME=Sociedade 67
```

A different deployment can use the same application image with another name.

## Application API

The Go service provides:

```text
GET    /api/tastings?q=term
POST   /api/tastings
GET    /api/tastings/{id}
PUT    /api/tastings/{id}
DELETE /api/tastings/{id}
```

Example create request:

```bash
curl -X POST http://127.0.0.1:8080/api/tastings \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "Example Dram",
    "distillery": "Example Distillery",
    "region": "Speyside",
    "country": "Scotland",
    "age": 12,
    "abv": 46,
    "caskType": "Ex-bourbon",
    "rating": 85,
    "nose": "Malt and fruit",
    "palate": "Honey and spice",
    "finish": "Warm and dry",
    "overall": "Balanced",
    "whatLearned": "Water opened the fruit character"
  }'
```

## Database

The application connects to PostgreSQL using `DATABASE_URL` and applies the initial schema on startup. The schema is in:

```text
cmd/server/migrations/001_initial.sql
```

Tasting records are stored in PostgreSQL. They are not stored in Git and must not be added to the repository.

## Tests and validation

Run application checks with Docker when Go is not installed locally:

```bash
docker run --rm \
  -v "$PWD":/src \
  -w /src \
  golang:1.24-alpine \
  sh -c 'go test ./... && go vet ./... && go build ./cmd/server'
```

The equivalent Makefile commands are:

```bash
make lint
make test
make build
make image-build
make docker-test
make k8s-validate
```

Build the production image:

```bash
docker build -t tasting-journals-app:local .
```

## Kubernetes manifests

The reusable Kubernetes base, when present, is rendered from:

```bash
kubectl kustomize deploy/base
```

The optional local PostgreSQL and application environment is rendered from:

```bash
kubectl kustomize optional/local-kubernetes
```

The optional environment is intended for Minikube or Kind only. It should create PostgreSQL and application resources together and use development-only credentials.

For Minikube, load a local image with:

```bash
minikube image load tasting-journals-app:local
```

For Kind:

```bash
kind load docker-image tasting-journals-app:local
```

Apply the optional environment:

```bash
kubectl apply -k optional/local-kubernetes
kubectl rollout status deployment/postgres -n tasting-journals-local
kubectl rollout status deployment/tasting-journals -n tasting-journals-local
```

## CI and releases

The repository uses GitHub Actions for:

- application linting and tests
- Go build validation
- Docker image publishing to GHCR
- semantic releases with Release Please
- dependency updates with Dependabot

Use Conventional Commits:

```text
feat: add tasting filters
fix: validate rating bounds
docs: improve local setup
chore: update CI configuration
```

Typical version behavior:

```text
fix:  v0.1.1
feat: v0.2.0
feat!: v1.0.0
```

Release Please generates `CHANGELOG.md` through a release pull request. Do not manually edit generated release metadata or commit production secrets.

## Production principles

- Keep `APP_NAME` and deployment-specific values outside reusable application code.
- Keep database credentials in the cluster secret-management system.
- Do not expose PostgreSQL directly to the public network.
- Protect write routes with authentication before production exposure.
- Keep `/healthz` and `/readyz` available for Kubernetes probes.
- Pin production image tags or digests; do not use `latest` as the deployment identifier.
