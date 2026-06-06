.PHONY: build check test

build:
	mkdir -p bin
	go build -o bin/repoforge .

test:
	go test ./...

check:
	test -z "$$(gofmt -l $$(git ls-files '*.go'))"
	go test -race ./...
	go vet ./...
