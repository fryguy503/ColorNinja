#!/usr/bin/env bash
# Native Linux/macOS build; CI supplies pinned Go, Node and npm.
set -euo pipefail
cd "$(dirname "$0")/.."
platform=$(go env GOOS)
case "$platform" in
  linux) tags=webkit2_41 ;;
  darwin) tags= ;;
  *) echo 'Use scripts/build.ps1 on Windows.' >&2; exit 1 ;;
esac
mkdir -p build/bin artifacts
export CGO_ENABLED=1
go mod download
go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
npm ci --prefix frontend
npm run format:check --prefix frontend
npm test --prefix frontend
npm run build --prefix frontend
go test -tags "$tags" -json ./... > artifacts/go-tests.jsonl
go vet -tags "$tags" ./...
go run ./cmd/artwork
"$(go env GOPATH)/bin/wails" build -skipbindings -s -trimpath -tags "$tags" -o ColorNinja
go build -trimpath -ldflags '-s -w' -o build/bin/colorninja-cli ./cmd/colorninja-cli
npm audit --prefix frontend
go install golang.org/x/vuln/cmd/govulncheck@v1.8.0
"$(go env GOPATH)/bin/govulncheck" -tags "desktop,production${tags:+,$tags}" . ./cmd/colorninja-cli
python3 scripts/package-native.py
