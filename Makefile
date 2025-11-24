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
	@echo "Generating documentation..."
	tfplugindocs generate

lint:
	golangci-lint run

fmt:
	gofmt -s -w -e .

tidy:
	go mod tidy

build-import-tool:
	cd tools/terraform-import && go build -v .

.PHONY: build install test testacc docs lint fmt tidy build-import-tool
