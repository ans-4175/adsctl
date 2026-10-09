.PHONY: build install fmt fmt-check vet lint test check tools
build:
	go build -trimpath -o bin/adsctl ./cmd/adsctl
install:
	go install ./cmd/adsctl
fmt:
	gofmt -w $$(find cmd internal -name '*.go')
fmt-check:
	@test -z "$$(gofmt -l $$(find cmd internal -name '*.go'))" || (gofmt -l $$(find cmd internal -name '*.go'); exit 1)
vet:
	go vet ./...
lint:
	golangci-lint run ./...
test:
	go test -race -coverprofile=coverage.out ./...
check: fmt-check vet lint test build
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
