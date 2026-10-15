.PHONY: build install fmt fmt-check vet lint test check tools plugin-check release-check release-snapshot
VERSION ?= dev
build:
	go build -trimpath -ldflags "-X adsctl/internal/cli.version=$(VERSION)" -o bin/adsctl ./cmd/adsctl
install:
	go install -trimpath -ldflags "-X adsctl/internal/cli.version=$(VERSION)" ./cmd/adsctl
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
check: fmt-check vet lint test build plugin-check
tools:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2
plugin-check:
	python3 scripts/check-plugin.py
release-check: plugin-check
	goreleaser check
release-snapshot: release-check
	goreleaser release --snapshot --clean --skip=publish
	python3 scripts/check-plugin.py --archives dist
