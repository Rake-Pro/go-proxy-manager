# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS build

# Pure-Go build (modernc.org/sqlite is cgo-free).
ENV CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

# Cache module downloads: copy manifests first, then download.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source.
COPY . .

# Honest version injection via ldflags.
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

RUN go build \
        -trimpath \
        -ldflags "-s -w \
            -X github.com/Rake-Pro/go-proxy-manager/internal/version.Version=${VERSION} \
            -X github.com/Rake-Pro/go-proxy-manager/internal/version.Commit=${COMMIT} \
            -X github.com/Rake-Pro/go-proxy-manager/internal/version.Date=${DATE}" \
        -o /out/gpm \
        ./cmd/gpm

# ---- final stage ----
# Named so the build workflows can exclude it from the layer cache
# (no-cache-filters: final): the apk upgrade below is only worth anything if it
# actually runs on every build. A cached layer silently pins whatever package
# versions existed when the RUN line last changed, and the release gate then
# fails on a CVE the repos already fixed (libexpat 2.8.3-r0 -> 2.8.4-r0, v1.0.32).
FROM alpine:3.24.1@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b AS final

# Runtime deps: app shells out to git; certs/tz for TLS and timestamps.
# apk upgrade first: the base image is digest-pinned and its packages lag
# security patches published to the 3.24 repos (e.g. openssl CVE-2026-14456
# fixed in 3.5.8-r0 with no new base image release), and the release gate
# fails on known HIGH/CRITICAL CVEs in the final image.
RUN apk upgrade --no-cache && apk add --no-cache git ca-certificates tzdata

# Non-root user.
RUN addgroup -S gpm && adduser -S -G gpm -H -h /data gpm

# Config / git-repo data dir owned by the non-root user.
RUN mkdir -p /data/config && chown -R gpm:gpm /data
VOLUME /data

COPY --from=build /out/gpm /usr/local/bin/gpm

USER gpm
WORKDIR /data

# Admin port (default 8081) + proxy ports.
EXPOSE 80 443 8081

# Health endpoint on the admin port.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8081/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/gpm"]
