#!/usr/bin/env bash
set -Eeuo pipefail

gofmt -w ./cmd/lunahub/main.go
go test ./...
go build -o /tmp/lunahub ./cmd/lunahub
bash -n install.sh
bash -n scripts/uninstall.sh

echo "OK"
