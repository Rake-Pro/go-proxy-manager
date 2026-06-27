# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26-alpine AS build

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
FROM alpine:latest

# Runtime deps: app shells out to git; certs/tz for TLS and timestamps.
RUN apk add --no-cache git ca-certificates tzdata

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
