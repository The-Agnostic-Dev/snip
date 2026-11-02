.PHONY: run build test vet

run:
	go run ./cmd/snip

build:
	go build -o bin/snip ./cmd/snip

test:
	go test ./...

vet:
	go vet ./...
