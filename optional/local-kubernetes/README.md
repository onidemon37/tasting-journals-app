# Local Kubernetes Example

This example is for local development only. It creates PostgreSQL and the application Deployment in the same Kustomize apply.

## Requirements

Install one of:

- Minikube
- Kind

Also install `kubectl` and Docker or another supported container runtime.

## Build and load the image

Build the application image from the repository root after the Dockerfile exists:

```bash
docker build -t tasting-journals-app:dev .
```

For Minikube:

```bash
minikube image load tasting-journals-app:dev
```

For Kind:

```bash
kind load docker-image tasting-journals-app:dev
```

## Deploy

```bash
kubectl apply -k optional/local-kubernetes
kubectl rollout status deployment/postgres -n tasting-journals-local
kubectl rollout status deployment/tasting-journals -n tasting-journals-local
kubectl port-forward service/tasting-journals 8080:80 -n tasting-journals-local
```

Then check:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
```

The local Secret contains development-only credentials. Never reuse them outside this example.

## Inspect and clean up

```bash
kubectl get pods -n tasting-journals-local
kubectl logs deployment/tasting-journals -n tasting-journals-local
kubectl delete -k optional/local-kubernetes
```
