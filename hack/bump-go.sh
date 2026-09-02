#!/usr/bin/env sh
# Bump the Go toolchain everywhere it is pinned: .go-version (read by CI),
# the builder image in Dockerfile (tag + digest) and test/stream/echo/Dockerfile.
# The docs quote the minimum language version from go.mod, not this pin, and
# internal/version/goversion_test.go fails CI if any of these drift apart.
# Usage: hack/bump-go.sh 1.27.2
set -eu
ver="${1:?usage: hack/bump-go.sh X.Y.Z}"
case "$ver" in *.*.*) ;; *) echo "want X.Y.Z, got $ver" >&2; exit 1 ;; esac
cd "$(dirname "$0")/.."
token=$(curl -fsS "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/golang:pull" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
digest=$(curl -fsSI -H "Authorization: Bearer $token" \
  -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json" \
  "https://registry-1.docker.io/v2/library/golang/manifests/${ver}-alpine" | tr -d '\r' | sed -n 's/^[Dd]ocker-[Cc]ontent-[Dd]igest: //p')
[ -n "$digest" ] || { echo "no manifest for golang:${ver}-alpine" >&2; exit 1; }
sed -i "s|^FROM golang:[0-9.]*-alpine@sha256:[0-9a-f]* AS build|FROM golang:${ver}-alpine@${digest} AS build|" Dockerfile
sed -i "s|^FROM golang:[0-9.]*-alpine AS build|FROM golang:${ver}-alpine AS build|" test/stream/echo/Dockerfile
printf '%s\n' "$ver" > .go-version
grep -n "golang:" Dockerfile test/stream/echo/Dockerfile; echo ".go-version: $(cat .go-version)"
