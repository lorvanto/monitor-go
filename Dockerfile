# syntax=docker/dockerfile:1
ARG NODE_IMAGE=node:24.22.0-alpine3.24@sha256:44fce696f38ba8c987c4db89954051000bbd05ffd8d8659ba15ed4a8602e937f
ARG GO_IMAGE=golang:1.26.9-alpine3.24@sha256:3082400e369fa24d5fc60bca20edab3f6d604e0c5a690ec66b295eff4dd87ade
ARG RUNTIME_IMAGE=alpine:3.24.2@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

FROM --platform=$BUILDPLATFORM ${NODE_IMAGE} AS frontend
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN --mount=type=cache,target=/root/.npm npm --prefix web ci
COPY . .
RUN npm --prefix web run build

FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
WORKDIR /src
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY locales ./locales
COPY web/embed.go ./web/embed.go
COPY --from=frontend /src/web/dist ./web/dist
ARG VERSION=0.4.0-dev
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -trimpath -ldflags "-X github.com/lorvanto/monitor-go/internal/buildinfo.Version=${VERSION}" -o /out/monitor ./cmd/monitor

FROM ${RUNTIME_IMAGE} AS runtime
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 monitor \
    && adduser -S -D -H -u 10001 -G monitor monitor \
    && mkdir -p /data \
    && chown 10001:10001 /data \
    && chmod 0700 /data
COPY --from=build /out/monitor /usr/local/bin/monitor
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md /usr/share/monitor-go/
COPY third_party/licenses /usr/share/monitor-go/third_party/licenses
ARG VERSION=0.4.0-dev
LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.title="Monitor Go" \
      org.opencontainers.image.description="Self-hosted Go monitoring with embedded React and SQLite" \
      org.opencontainers.image.source="https://github.com/lorvanto/monitor-go" \
      org.opencontainers.image.licenses="Apache-2.0"
USER 10001:10001
WORKDIR /data
VOLUME ["/data"]
EXPOSE 5080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:5080/readyz || exit 1
ENTRYPOINT ["/usr/local/bin/monitor"]
CMD ["-data", "/data", "-listen", ":5080", "-collect-host=false"]
