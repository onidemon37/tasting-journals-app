# Deployment Manifests

The `base/` directory contains reusable Kubernetes resources for the tasting-journals application.

It expects:

- an image that listens on port `8080`
- `/healthz` and `/readyz` endpoints
- a Secret named `tasting-journals-database` with a `DATABASE_URL` key
- a PostgreSQL service named `postgres` in the same namespace

Use `optional/local-kubernetes/` for a complete local PostgreSQL and application deployment.
