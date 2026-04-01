.PHONY: build test lint fmt security clean release-snapshot generate sync-spec generate-check

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
  -X github.com/weside-ai/weside-cli/cmd.version=$(VERSION) \
  -X github.com/weside-ai/weside-cli/cmd.commit=$(COMMIT) \
  -X github.com/weside-ai/weside-cli/cmd.date=$(DATE)
WESIDE_CORE ?= ../weside-core

build:
	go build -ldflags "$(LDFLAGS)" -o weside .

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	golangci-lint run
	gofumpt -l -d .

fmt:
	gofumpt -w .

security:
	govulncheck ./...

sync-spec:
	cp $(WESIDE_CORE)/apps/backend/weside-client-openapi.json internal/api/
	go generate ./...

generate:
	go generate ./...

generate-check: generate
	@git diff --exit-code internal/api/types.gen.go || \
		(echo "ERROR: Generated Go types are out of date. Run 'make sync-spec'" && exit 1)

clean:
	rm -f weside coverage.out

release-snapshot:
	goreleaser release --snapshot --clean
