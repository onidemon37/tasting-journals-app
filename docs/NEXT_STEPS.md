# Sociedade 67: Next Steps

This document is the working queue for the next development session. The current application is a local, PostgreSQL-backed tasting journal with browser CRUD, Docker support, and local Kubernetes manifests.

## Current State

Completed:

- Go HTTP application
- PostgreSQL schema and startup migration
- Tasting create, read, update, delete, and search API
- Browser create, edit, and delete workflows
- Health and database readiness endpoints
- Docker Compose development environment
- Production multi-stage Docker image
- Minikube/Kind local Kubernetes example
- Configurable `APP_NAME` branding
- CI, GHCR image publishing, Release Please, and Dependabot configuration

Still intentionally local or incomplete:

- Edit and delete have no authentication yet.
- Production Flux integration has not been added.
- Staging and production deployment overlays are not complete.
- Image promotion Pull Requests are not complete.
- `sociedade67.com` DNS and TLS are not configured.
- The application currently has no user or member model.
- Login and invitation management are not implemented.

The application is intended to become part of a broader audit-oriented platform. Auditability is therefore a first-class requirement across application behavior, infrastructure, deployments, identities, secrets, and operational access.

## Tomorrow: Finish Stage 1

### 1. Improve the application foundation

- Add focused Go unit tests for validation, tag parsing, and handlers.
- Add PostgreSQL integration tests.
- Add an append-only PostgreSQL `audit_events` table.
- Record transactional audit events for tasting creation, edits, and deletion.
- Include actor, action, resource, timestamp, request ID, result, and relevant before/after data.
- Redact passwords, tokens, cookies, private keys, database URLs, and other secrets from audit data.
- Add request IDs and correlate audit events with logs and traces.
- Add administrator-only audit access; normal users must not edit or delete audit records.
- Add tests proving audit records are written with business changes and cannot be removed by the application role.
- Define a platform-wide audit event schema shared by future Sociedade 67 modules.
- Use stable event names such as `tasting.created`, `user.invited`, `deployment.promoted`, and `secret.accessed`.
- Record who, what, when, where, outcome, target resource, request ID, trace ID, and source system.
- Distinguish actor identity, service identity, administrator identity, and automated-controller identity.
- Record authorization decisions, including denied access attempts, without recording secrets or sensitive payloads.
- Write business changes and their audit events in the same PostgreSQL transaction.
- Make audit records append-only to the application role and restrict reads to authorized administrators or auditors.
- Add an audit export format such as JSON Lines with a documented schema version.
- Add an audit viewer with filtering by actor, action, resource, outcome, and time range.
- Define clock and timestamp rules using UTC and consistent server-side timestamps.
- Define retention, deletion, legal hold, and privacy rules before collecting personal data.
- Add structured JSON application logs with request method, route, status, duration, and request ID.
- Add application metrics for HTTP requests, latency, response status, database queries, and database errors.
- Add a protected or internal `/metrics` endpoint for Prometheus.
- Add OpenTelemetry instrumentation for HTTP requests and PostgreSQL queries where practical.
- Export telemetry through the existing cluster collector and correlate trace IDs with request IDs.
- Define audit, log, metric, and trace retention and backup requirements.
- Add graceful shutdown for the HTTP server.
- Add database connection timeouts and query context limits.
- Add migration version tracking instead of running one combined migration forever.
- Improve error pages and form validation messages.
- Add pagination and filters for region, distillery, cask type, rating, and tags.
- Add sorting options to the tasting list.

### 1a. Dashboard and observability integration

- Add a Kubernetes `ServiceMonitor` or `PodMonitor` following the cluster's existing Prometheus conventions.
- Create a Grafana dashboard for request rate, latency, errors, health status, and PostgreSQL connectivity.
- Add dashboard panels for tasting activity, including entries created, updated, and deleted.
- Add alerts for application unavailability, elevated error rates, slow responses, and database connection failures.
- Ensure credentials, cookies, tasting content, and personal data never appear in logs or telemetry.
- Define trace sampling appropriate for a home lab.
- Add PostgreSQL permissions that prevent application users from deleting audit records.
- Create an audit-reader role for administrator access.
- Evaluate hash chaining for tamper-evident audit events after the basic audit trail is tested.

### 2. Protect write operations

- Use invitation-only authentication for the first release.
- Decide whether the application is private-read/private-write or public-read/private-write.
- Add users, invitations, sessions, and roles to PostgreSQL.
- Allow only an existing administrator to create invitations.
- Generate cryptographically random, single-use invitation tokens.
- Store only a hash of each invitation token; never store the raw token.
- Set invitation expiry and revoke unused invitations.
- Send invitation links through a configured email provider or display them only in a protected administrator workflow during local development.
- Allow invited users to set a password or use a passwordless login flow.
- Hash passwords with Argon2id or bcrypt; never store plaintext passwords.
- Use secure, HttpOnly, SameSite session cookies.
- Add logout, session expiry, session revocation, and login rate limiting.
- Protect create, edit, delete, invitation, and administration routes.
- Add CSRF protection for browser forms.
- Keep GitHub and database credentials server-side.
- Add request size limits and input validation.

### 2a. Vulnerability management and security proof

- Run `govulncheck ./...` for Go dependency and standard-library vulnerabilities.
- Run `go vet ./...` and a strict formatter check in CI.
- Run Trivy or Grype against the production container image.
- Generate an SBOM with Syft and retain it with each image release.
- Sign released images with Cosign and verify signatures before production deployment.
- Scan Git history and pull requests for secrets with Gitleaks.
- Scan Kubernetes manifests with Kubescape, KubeLinter, or Trivy config scanning.
- Add dependency updates through Dependabot or Renovate.
- Pin GitHub Actions to reviewed major versions or commit SHAs according to repository practice.
- Run OWASP ZAP baseline checks against the staging HTTP endpoint.
- Run authenticated DAST checks after invitation-only login exists.
- Add tests for authorization boundaries, CSRF, session expiry, invitation reuse, and rate limits.
- Produce a release security report containing dependency scan, image scan, SBOM, signature, and manifest-scan results.
- Define severity thresholds that block releases, for example critical vulnerabilities with a fix available.
- Document accepted-risk exceptions with an owner, reason, expiry date, and compensating control.
- Test PostgreSQL backup restoration and secret rotation.

### 2b. Audit evidence and control validation

- Create an audit event taxonomy and ownership document.
- Map important actions to security controls and evidence sources.
- Capture application audit events for authentication, invitations, tasting changes, permissions, and administrative actions.
- Capture GitHub evidence for pull requests, approvals, branch protection, releases, workflow runs, and package publication.
- Capture Flux evidence for source revisions, reconciliations, health state, image updates, and deployment failures.
- Capture Kubernetes evidence for API access, workload changes, events, RBAC decisions, and namespace activity.
- Capture Vault evidence for authentication, policy changes, secret reads, secret writes, and administrative operations.
- Capture PostgreSQL evidence for role changes, schema migrations, privileged access, and audit-table changes.
- Keep audit evidence separate from high-volume operational logs while correlating both with request and trace IDs.
- Forward audit events to a restricted, durable store separate from the application database when the platform matures.
- Protect audit transport with authenticated encryption and restrict collectors with least privilege.
- Add integrity checks such as signed batches or hash chaining for exported evidence.
- Record evidence source, collection time, schema version, and integrity status.
- Monitor gaps in collection and alert when an audit source stops reporting.
- Test that clock skew, retries, duplicate delivery, and out-of-order events do not corrupt the audit trail.
- Create a documented evidence access process with approval, reason, scope, and expiration.
- Never use ordinary application logs as the sole source of compliance evidence.

### 3. Complete the reusable application repository

- Confirm the final repository name: `tasting-journals-app`.
- Update all image names and documentation consistently.
- Add a complete README with architecture, local setup, API, migrations, and operations.
- Add `docker compose` development commands.
- Add an example tasting dataset only if clearly marked as demo data.
- Keep all real tasting data out of Git.

## Stage 1 Deployment

### 4. Create `sociedade67-cd-production`

This repository should contain only Sociedade 67 deployment configuration:

- Production Kustomize overlays
- Image repository and image policy
- Production image promotion configuration
- Sociedade 67 branding values
- Production hostname configuration
- Environment-specific resource settings

Do not copy application source code into this repository.

### 5. Integrate with `k8s-gitops-cluster`

Add the Flux resources following existing cluster conventions:

- Application GitRepository
- Application Kustomization
- Namespace configuration
- Dependencies on PostgreSQL and networking resources
- Secret references using the existing secret-management system
- Production HTTPRoute
- Certificate configuration

Validate with:

```bash
kubectl kustomize kubernetes/apps
```

### 6. Add staging and production flow

Target behavior:

```text
merge application PR
  -> CI passes
  -> image tagged with commit SHA
  -> image pushed to GHCR
  -> staging updates automatically

create semantic release
  -> versioned image is published
  -> production promotion PR is created
  -> human reviews and merges
  -> Flux deploys production
```

Keep staging and production image policies separate.

### 7. Configure domain and TLS

- Register or confirm `sociedade67.com`.
- Create the required DNS records for the cluster ingress.
- Add `sociedade67.com` and `whisky.sociedade67.com` only when the route design is finalized.
- Issue certificates through the existing cert-manager and Cloudflare DNS01 setup.
- Verify HTTPS and certificate renewal.

## Stage 2: Bottle Pictures

Stage 2 adds images to whisky entries without replacing PostgreSQL as the canonical application database.

### Data model

Add a `bottle_images` table with:

- image ID
- whisky ID
- object-storage key or URL
- original filename
- content type
- file size
- width and height
- alt text
- primary image flag
- created timestamp

### Storage

Choose an object-storage solution compatible with the home cluster. Do not store uploaded images in the container filesystem.

Evaluate:

- S3-compatible storage
- existing cluster storage
- backup and restore behavior
- image size limits
- private versus public object access

### Application

- Add authenticated image upload.
- Add image deletion and replacement.
- Add primary-image selection.
- Add thumbnail or responsive-image handling.
- Add safe content-type and file-size validation.
- Add image display to tasting detail pages.
- Keep image metadata separate from tasting-note fields.

### Security

- Never trust the uploaded filename.
- Validate actual file content, not only the extension.
- Restrict image formats and maximum size.
- Prevent arbitrary path traversal.
- Keep storage credentials server-side.
- Add tests for invalid and oversized uploads.

## Stage 3: Distilleries and Maps

Stage 3 introduces reusable distillery entities and geographic information.

### Data model

Add a `distilleries` table with:

- name
- country
- region
- town or locality
- latitude
- longitude
- website
- description
- created and updated timestamps

Change whisky records to reference `distillery_id` rather than storing only free-form text. Preserve a migration path for existing data.

### Application

- Add distillery create/edit/view workflows.
- Link tastings to distilleries.
- Add distillery filtering.
- Add a distillery detail page.
- Add geographic metadata validation.
- Add map display only after the data model is stable.

### Maps

Choose a map provider and document:

- API key requirements
- usage limits
- privacy implications
- tile licensing
- offline or low-connectivity behavior

Do not add map infrastructure before the distillery data model is complete.

## Future Sociedade 67 Areas

After whisky Stage 3, consider:

- cigar tasting notes
- whisky and cigar pairings
- club member profiles
- private notes versus shared notes
- bottle inventory
- tasting sessions
- recommendations
- event notes
- activity history

These should be introduced as separate application modules that share authentication and platform services.

## Operational Work

- Deploy and validate the Grafana dashboard in the observability stack.
- Confirm Prometheus scrapes the application metrics endpoint.
- Confirm logs are collected and searchable through the existing log aggregation system.
- Confirm traces are visible through the selected telemetry backend.
- Document the relationship between metrics, logs, traces, and request IDs for troubleshooting.
- Add PostgreSQL backup and restore documentation.
- Test restore procedures in a non-production database.
- Add database metrics and alerting.
- Document rollback to a previous image digest.
- Document secret rotation.
- Add dependency and base-image update policy.
- Add production resource sizing after observing staging.
- Add NetworkPolicy after confirming required traffic paths.
- Add a PodDisruptionBudget only when replica count and availability requirements justify it.
- Centralize audit evidence only after the event schema and retention policy are stable.
- Create dashboards for audit-event volume, denied actions, missing sources, and collection latency.
- Alert on audit pipeline failure, unexpected privileged actions, policy changes, and disabled logging.
- Test an end-to-end audit investigation from user action to application event, log, trace, Git change, and Kubernetes reconciliation.
- Perform periodic access reviews for administrators, auditors, CI identities, Vault policies, and Kubernetes service accounts.
- Document evidence export, verification, retention, restoration, and secure destruction procedures.
- Maintain a control-to-evidence matrix for future customer or regulatory audits.

## Definition of Done for the Next Session

- [ ] Stage 1 authentication decision made.
- [ ] Go tests added and passing.
- [ ] Audit events are recorded transactionally for tasting changes.
- [ ] Structured logs, Prometheus metrics, and request IDs are implemented.
- [ ] Initial OpenTelemetry instrumentation approach is implemented or documented.
- [ ] Platform audit event schema and taxonomy are documented.
- [ ] Application audit events are transactional, append-only, and access-controlled.
- [ ] GitHub, Flux, Kubernetes, Vault, and PostgreSQL evidence sources are mapped.
- [ ] Audit retention, privacy, and legal-hold rules are documented.
- [ ] Audit evidence integrity and collection-gap monitoring are designed.
- [ ] Staging and production CD layout agreed.
- [ ] Flux integration structure added without production deployment yet.
- [ ] Domain and TLS prerequisites documented.
- [ ] Stage 2 image storage decision recorded.
- [ ] Application metrics and Grafana dashboard design agreed.
- [ ] Prometheus scraping and alerting approach agreed.
- [ ] Structured logging and log aggregation approach agreed.
- [ ] OpenTelemetry tracing approach agreed.
- [ ] Invitation-only login design implemented and tested.
- [ ] Go vulnerability scanning is part of CI.
- [ ] Container image scanning and SBOM generation are part of releases.
- [ ] Container image signing and production verification are implemented.
- [ ] Kubernetes manifest security scanning is part of CI.
- [ ] Staging DAST scan is configured.
- [ ] Security exceptions have an owner and expiry date.
- [ ] No secrets or personal tasting data committed.
