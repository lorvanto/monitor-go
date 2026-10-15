.PHONY: build web test run docker-build test-docker
web:
	npm --prefix web ci
	npm --prefix web run build
build: web
	go build -trimpath -o bin/monitor ./cmd/monitor
	go build -trimpath -o bin/agent ./cmd/agent
run: build
	./bin/monitor -data data -listen 127.0.0.1:5080
test: web
	npm --prefix web test
	go test -race ./...
	go vet ./...
docker-build:
	docker compose build
test-docker:
	node scripts/test-docker.mjs
