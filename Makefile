LAST_TAG=$(shell git describe --abbrev=0 --tags)
CURR_SHA=$(shell git rev-parse --verify HEAD)

LDFLAGS=-ldflags "-s -w -X main.version=$(LAST_TAG)"

.PHONY: release symbols gob json deps

all: build

# make release tag=v0.4.3
release:
	git tag $(tag)
	git push origin $(tag)

deps:
	go install golang.org/x/tools/gopls@v0.16.2
	go install github.com/gnolang/gno/gnovm/cmd/gno@master

build:
	GOOS=$(os) GOARCH=$(arch) go build ${LDFLAGS} -o bin/$(exe) ./cmd/gnols

gob:
	go run cmd/gen/main.go --root-dir "/Users/jdkato/Documents/Code/Gno/gno" 

json:
	go run cmd/gen/main.go --root-dir "/Users/jdkato/Documents/Code/Gno/gno" --format json

lint:
	go run github.com/golangci/golangci-lint/cmd/golangci-lint@v1.59.1 run ./...
