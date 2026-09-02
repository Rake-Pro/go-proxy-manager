BINARY      := gpm
PKG         := ./cmd/gpm
BIN_DIR     := bin
VERSION_PKG := github.com/Rake-Pro/go-proxy-manager/internal/version

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

IMAGE   ?= ghcr.io/rake-pro/go-proxy-manager

LDFLAGS := -s -w \
	-X $(VERSION_PKG).Version=$(VERSION) \
	-X $(VERSION_PKG).Commit=$(COMMIT) \
	-X $(VERSION_PKG).Date=$(DATE)

export CGO_ENABLED := 0

.PHONY: build run test vet lint vuln tidy docker clean

build:
	mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(PKG)

run:
	go run -ldflags "$(LDFLAGS)" $(PKG)

test:
	go test ./...

vet:
	go vet ./...

lint:
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...

tidy:
	go mod tidy

docker:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t $(IMAGE):$(VERSION) .

clean:
	rm -rf $(BIN_DIR)

# Bump the pinned Go toolchain everywhere (CI, builder images): make bump-go VERSION=1.27.2
bump-go:
	hack/bump-go.sh $(VERSION)
