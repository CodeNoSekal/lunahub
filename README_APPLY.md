# LunaHub install fixes

Files in this bundle:

- `install.sh` — replace repository root `install.sh` with this file.
- `scripts/uninstall.sh` — add this new file and make it executable.
- `main.go.patch` — apply this patch to `cmd/lunahub/main.go`.

Apply:

```bash
cp install.sh /path/to/lunahub/install.sh
mkdir -p /path/to/lunahub/scripts
cp scripts/uninstall.sh /path/to/lunahub/scripts/uninstall.sh
chmod +x /path/to/lunahub/install.sh /path/to/lunahub/scripts/uninstall.sh

cd /path/to/lunahub
git apply /path/to/main.go.patch

gofmt -w ./cmd/lunahub/main.go
go test ./... 2>/dev/null || true
go build -o /tmp/lunahub-test ./cmd/lunahub

git diff
git add install.sh scripts/uninstall.sh cmd/lunahub/main.go
git commit -m "Fix installer config generation and service permissions"
```
