export CGO_ENABLED := "0"

default:
    @just --list

build:
    go build -trimpath -o dist/runner ./cmd/runner

test:
    CGO_ENABLED=1 go test -race ./...

check:
    test -z "$(gofmt -l cmd internal)"
    go vet ./...

windows:
    GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o dist/runner-windows-amd64.exe ./cmd/runner

install:
    go install ./cmd/runner

release:
    bash scripts/release.sh

ci-deps:
    bash scripts/ci-deps.sh
