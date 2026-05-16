#!/usr/bin/env bash
set -Eeuo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

gofmt -w ./cmd/lunahub/main.go
go test ./...
go build -o /tmp/lunahub ./cmd/lunahub
bash -n install.sh
bash -n scripts/uninstall.sh

echo "OK: dev checks passed"
