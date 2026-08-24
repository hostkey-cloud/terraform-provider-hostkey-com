.PHONY: build install test testacc fmt vet lint tidy validate docs release

HOSTNAME=$(shell go env GOHOSTOS)
ARCH=$(shell go env GOHOSTARCH)
VERSION?=dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o terraform-provider-hostkey-com .

install:
	go install -ldflags "-X main.version=$(VERSION)"

test:
	go test ./... -count=1 -timeout 5m

testacc:
	go test -tags=acceptance ./internal/provider -v -count=1 -timeout 180m -run TestAcc

fmt:
	go fmt ./...

vet:
	go vet ./...

tidy:
	go mod tidy

lint:
	golangci-lint run ./...

validate:
	@terraform fmt -check -recursive examples || (echo "run: terraform fmt -recursive examples" && exit 1)

docs:
	@echo "Registry markdown lives under docs/. Optional: go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs@latest generate"

release:
	goreleaser release --snapshot --clean
