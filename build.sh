#!/bin/bash
# Atavus Agent Build Script
# Cross-compiles for Windows and macOS from Linux

set -e

OUTDIR="/www/wwwroot/atavus.ai/downloads"
VERSION="1.0.0"

echo "═══ Atavus Agent Build v${VERSION} ═══"
echo ""

cd "$(dirname "$0")"

# ── Linux (build target, debugging) ──
echo "→ Linux amd64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.platform=linux -X main.version=${VERSION}" \
  -o "${OUTDIR}/agent/atavus-agent-linux" .
echo "  ✅ ${OUTDIR}/agent/atavus-agent-linux"

# ── Windows amd64 (most common) ──
echo "→ Windows amd64..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build \
  -ldflags="-s -w -X main.platform=windows -X main.version=${VERSION} -H windowsgui" \
  -o "${OUTDIR}/agent/atavus-agent-windows-amd64.exe" .
echo "  ✅ ${OUTDIR}/agent/atavus-agent-windows-amd64.exe"

# ── Windows 386 (older systems) ──
echo "→ Windows 386..."
CGO_ENABLED=0 GOOS=windows GOARCH=386 go build \
  -ldflags="-s -w -X main.platform=windows -X main.version=${VERSION} -H windowsgui" \
  -o "${OUTDIR}/agent/atavus-agent-windows-386.exe" .
echo "  ✅ ${OUTDIR}/agent/atavus-agent-windows-386.exe"

# ── macOS amd64 (Intel) ──
echo "→ macOS amd64..."
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build \
  -ldflags="-s -w -X main.platform=macos -X main.version=${VERSION}" \
  -o "${OUTDIR}/agent/atavus-agent-macos-amd64" .
echo "  ✅ ${OUTDIR}/agent/atavus-agent-macos-amd64"

# ── macOS arm64 (Apple Silicon) ──
echo "→ macOS arm64..."
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build \
  -ldflags="-s -w -X main.platform=macos -X main.version=${VERSION}" \
  -o "${OUTDIR}/agent/atavus-agent-macos-arm64" .
echo "  ✅ ${OUTDIR}/agent/atavus-agent-macos-arm64"

echo ""
echo "═══ All builds complete ═══"
ls -lh "${OUTDIR}/agent/"
