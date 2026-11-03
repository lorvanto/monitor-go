#!/bin/sh
# Build the installable Linux Agent packages from this checkout and its Go lockfile.
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
agent_output=${1:-bin/agents}
agent_version=${MONITOR_BUILD_VERSION:-0.4.0-dev}
mkdir -p "$agent_output"

for agent_arch in amd64 arm64; do
    agent_target=$agent_output/agent-linux-$agent_arch
    CGO_ENABLED=0 GOOS=linux GOARCH=$agent_arch go build -trimpath \
        -ldflags "-X github.com/lorvanto/monitor-go/internal/buildinfo.Version=$agent_version" \
        -o "$agent_target.tmp" ./cmd/agent
    mv "$agent_target.tmp" "$agent_target"
done
