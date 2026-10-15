VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || printf '0.4.0-dev')
export MONITOR_BUILD_VERSION := $(VERSION)

.PHONY: build web test run docker-build test-docker
web:
	npm --prefix web ci
	npm --prefix web run build
build: web
	go build -trimpath -ldflags "-X github.com/lorvanto/monitor-go/internal/buildinfo.Version=$$MONITOR_BUILD_VERSION" -o bin/monitor ./cmd/monitor
	go build -trimpath -ldflags "-X github.com/lorvanto/monitor-go/internal/buildinfo.Version=$$MONITOR_BUILD_VERSION" -o bin/agent ./cmd/agent
run: build
	./bin/monitor -data data -listen 127.0.0.1:5080
test: web
	npm --prefix web test
	go test -race ./...
	go vet ./...
docker-build:
	MONITOR_VERSION="$$MONITOR_BUILD_VERSION" docker compose build
test-docker:
	MONITOR_VERSION="$$MONITOR_BUILD_VERSION" node scripts/test-docker.mjs
