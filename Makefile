.DEFAULT_GOAL := help
.ONESHELL:

SHELL := /bin/bash
export PATH := /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:$(PATH)

APP_IMAGE ?= tasting-journals-app:dev
GO_IMAGE ?= golang:1.24-alpine
GO_VERSION ?= 1.24.7
GO_INSTALL_DIR ?= $(HOME)/.local/go
GO_ARCHIVE ?= go$(GO_VERSION).linux-amd64.tar.gz
COMPOSE := docker compose
K8S_NAMESPACE ?= tasting-journals-local

.PHONY: help install-go fmt fmt-check lint test vet build image-build image-run docker-test k8s-render k8s-validate k8s-load k8s-apply k8s-status clean

help:
	@printf '%s\n' \
		'Available targets:' \
		'  make install-go   Install the pinned Go toolchain with mise or Homebrew' \
		'  make fmt           Format Go source' \
		'  make fmt-check     Check Go formatting' \
		'  make lint          Run Go vet and formatting checks' \
		'  make test          Run Go tests' \
		'  make build         Build the Go binary' \
		'  make image-build   Build the production Docker image' \
		'  make image-run     Run app and PostgreSQL with Docker Compose' \
		'  make docker-test   Smoke-test the Docker Compose application' \
		'  make k8s-render    Render reusable Kubernetes manifests' \
		'  make k8s-validate  Render all local Kubernetes manifests' \
		'  make k8s-load      Load the image into Minikube or Kind' \
		'  make k8s-apply      Apply the local Kubernetes example' \
		'  make k8s-status     Show local Kubernetes resources' \
		'  make clean         Remove generated local output'

install-go:
	@if command -v go >/dev/null 2>&1; then \
		printf 'Go is already installed: '; go version; \
	elif command -v mise >/dev/null 2>&1; then \
		mise install go@$(GO_VERSION); \
		printf '%s\n' 'Go installed through mise.'; \
		printf '%s\n' 'Run: mise exec -- make lint test build'; \
	elif command -v brew >/dev/null 2>&1; then \
		brew install go; \
	else \
		printf '%s\n' 'Downloading Go into $(GO_INSTALL_DIR)...'; \
		mkdir -p "$(GO_INSTALL_DIR)"; \
		if command -v curl >/dev/null 2>&1; then \
			curl --fail --location --output "/tmp/$(GO_ARCHIVE)" "https://go.dev/dl/$(GO_ARCHIVE)"; \
		elif command -v python3 >/dev/null 2>&1; then \
			python3 -c 'import sys, urllib.request; urllib.request.urlretrieve(sys.argv[1], sys.argv[2])' "https://go.dev/dl/$(GO_ARCHIVE)" "/tmp/$(GO_ARCHIVE)"; \
		else \
			printf '%s\n' 'curl or python3 is required for automatic Go installation.' >&2; exit 1; \
		fi; \
		rm -rf "$(GO_INSTALL_DIR)/go"; \
		tar -C "$(GO_INSTALL_DIR)" -xzf "/tmp/$(GO_ARCHIVE)"; \
		rm -f "/tmp/$(GO_ARCHIVE)"; \
		printf '%s\n' 'Go installed.'; \
		printf '%s\n' 'Run: export PATH="$(GO_INSTALL_DIR)/go/bin:$$PATH"'; \
		printf '%s\n' 'Then run: make lint test build'; \
	fi

fmt:
	@if command -v go >/dev/null 2>&1; then \
		gofmt -w $$(find cmd -type f -name '*.go'); \
	else \
		docker run --rm -v "$$PWD":/src -w /src $(GO_IMAGE) sh -c 'gofmt -w $$(find cmd -type f -name "*.go")'; \
	fi

fmt-check:
	@files=$$(find cmd -type f -name '*.go'); \
	if command -v go >/dev/null 2>&1; then \
		unformatted=$$(gofmt -l $$files); \
	else \
		unformatted=$$(docker run --rm -v "$$PWD":/src -w /src $(GO_IMAGE) sh -c 'gofmt -l $$(find cmd -type f -name "*.go")'); \
	fi; \
	if [[ -n "$$unformatted" ]]; then printf 'Unformatted Go files:\n%s\n' "$$unformatted"; exit 1; fi

lint: fmt-check vet

vet:
	@if command -v go >/dev/null 2>&1; then go vet ./...; else docker run --rm -v "$$PWD":/src -w /src $(GO_IMAGE) sh -c 'go vet ./...'; fi

test:
	@if command -v go >/dev/null 2>&1; then go test ./...; else docker run --rm -v "$$PWD":/src -w /src $(GO_IMAGE) sh -c 'go test ./...'; fi

build:
	@if command -v go >/dev/null 2>&1; then CGO_ENABLED=0 go build -o /tmp/tasting-journals ./cmd/server; else docker run --rm -v "$$PWD":/src -w /src $(GO_IMAGE) sh -c 'CGO_ENABLED=0 go build -o /tmp/tasting-journals ./cmd/server'; fi

image-build:
	docker build --tag $(APP_IMAGE) .

image-run: image-build
	$(COMPOSE) up --build

docker-test: image-build
	$(COMPOSE) up --detach --build
	trap '$(COMPOSE) down --remove-orphans' EXIT
	for attempt in $$(seq 1 30); do \
		if curl --fail --silent http://127.0.0.1:8080/healthz >/dev/null; then break; fi; \
		if [[ $$attempt -eq 30 ]]; then $(COMPOSE) logs; exit 1; fi; \
		sleep 2; \
	done
	curl --fail --silent http://127.0.0.1:8080/healthz
	printf '\nDocker smoke test passed.\n'

k8s-render:
	kubectl kustomize deploy/base >/dev/null

k8s-validate: k8s-render
	kubectl kustomize optional/local-kubernetes >/dev/null
	printf '%s\n' 'Kubernetes manifests are valid.'

k8s-load: image-build
	if command -v minikube >/dev/null 2>&1 && minikube status >/dev/null 2>&1; then \
		minikube image load $(APP_IMAGE); \
	elif command -v kind >/dev/null 2>&1; then \
		kind load docker-image $(APP_IMAGE); \
	else \
		printf '%s\n' 'No running Minikube profile or Kind cluster found.' >&2; exit 1; \
	fi

k8s-apply: k8s-load k8s-validate
	kubectl apply -k optional/local-kubernetes
	kubectl rollout status deployment/postgres --namespace $(K8S_NAMESPACE) --timeout=180s
	kubectl rollout status deployment/tasting-journals --namespace $(K8S_NAMESPACE) --timeout=180s

k8s-status:
	kubectl get pods,svc,deploy --namespace $(K8S_NAMESPACE)

clean:
	rm -f /tmp/tasting-journals
	$(COMPOSE) down --remove-orphans
