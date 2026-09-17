GO ?= go
VERSION ?= dev

.PHONY: build test audit bench release
build:
	$(GO) build -trimpath -ldflags '-s -w -X main.version=$(VERSION)' -o bin/atlo ./cmd/atlo

test:
	$(GO) test -race ./...
	$(GO) vet ./...
	python3 -B -m unittest discover -s scripts -p 'test_*.py'

audit:
	$(GO) mod verify
	$(GO) run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

bench:
	$(GO) test ./internal/cli -run '^$$' -bench . -benchmem

release:
	python3 scripts/release.py --go '$(GO)' --version '$(VERSION)'
