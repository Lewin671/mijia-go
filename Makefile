.PHONY: build test check package
VERSION ?= dev

build:
	go build -trimpath -ldflags "-s -w -X github.com/Lewin671/mijia-go/internal/cli.Version=$(VERSION)" -o bin/mijia ./cmd/mijia

test:
	go test -race ./...

check:
	go vet ./...
	go mod verify

package:
	bash scripts/package.sh "$(VERSION)"
