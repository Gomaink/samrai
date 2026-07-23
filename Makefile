APP := samrai
VERSION ?= 0.2.0-rc.8
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X samrai/internal/version.Version=$(VERSION) \
	-X samrai/internal/version.Commit=$(COMMIT) \
	-X samrai/internal/version.Date=$(BUILD_DATE)

.PHONY: run build test test-race fmt vet tidy check web-install web-dev web-build docker-build clean

run:
	go run ./cmd/server

web-install:
	cd web && npm ci

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

build: web-build
	mkdir -p bin
	go build -tags=nodynamic -trimpath -ldflags="$(LDFLAGS)" -o bin/$(APP) ./cmd/server

test:
	go test ./...

test-race:
	go test -race ./...

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

vet:
	go vet ./...

tidy:
	go mod tidy

check: web-build fmt vet test

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(APP):$(VERSION) .

clean:
	rm -rf bin coverage.out web/node_modules web/node_modules/.tmp
