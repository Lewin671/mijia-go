#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
version="${1:?usage: scripts/package.sh VERSION}"
if [[ ! "$version" =~ ^[a-zA-Z0-9][a-zA-Z0-9._-]*$ ]]; then
  echo 'invalid release version' >&2
  exit 2
fi
mkdir -p dist
for target_os in darwin linux windows; do
  for target_arch in amd64 arm64; do
    name="mijia-go_${version}_${target_os}_${target_arch}"
    stage="dist/$name"
    # This exact path is owned by this packaging script; never reuse stale files.
    rm -rf "$stage"
    rm -f "dist/$name.zip" "dist/$name.tar.gz"
    mkdir -p "$stage"
    binary=mijia
    if [[ "$target_os" == windows ]]; then binary=mijia.exe; fi
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath \
      -ldflags "-s -w -X github.com/Lewin671/mijia-go/internal/cli.Version=$version" \
      -o "$stage/$binary" ./cmd/mijia
    cp LICENSE THIRD_PARTY_NOTICES.md README.md README.zh-CN.md CONTRIBUTING.md SECURITY.md CHANGELOG.md AGENTS.md "$stage/"
    cp -R skills docs "$stage/"
    if [[ "$target_os" == windows ]]; then
      (cd dist && zip -q -r "$name.zip" "$name")
    else
      tar -C dist -czf "dist/$name.tar.gz" "$name"
    fi
  done
done
python3 - <<'PY'
from pathlib import Path
import hashlib
archives = sorted(p for p in Path('dist').iterdir() if p.name.endswith(('.tar.gz', '.zip')))
Path('dist/checksums.txt').write_text(''.join(f'{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n' for p in archives))
PY
