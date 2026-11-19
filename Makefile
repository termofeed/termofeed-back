.PHONY: all run test lint gen build docker loadtest

all: lint test build

run:
	go run ./cmd/termofeed

test:
	go test -v -cover ./...

lint:
	go vet ./...

gen:
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.2 -config oapi-codegen.yaml api/openapi.yaml

build:
	go build -trimpath -ldflags "-s -w" -o bin/termofeed ./cmd/termofeed

docker:
	docker build -t termofeed-back:latest .

loadtest:
	go run ./cmd/loadtest/main.go
