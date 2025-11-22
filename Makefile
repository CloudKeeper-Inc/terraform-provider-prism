default: install

build:
	go build -v ./...

install: build
	go install -v ./...

test:
	go test -v -cover -timeout=120s -parallel=4 ./...

testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

docs:
	go generate ./...

lint:
	golangci-lint run

fmt:
	gofmt -s -w -e .

tidy:
	go mod tidy

.PHONY: build install test testacc docs lint fmt tidy
