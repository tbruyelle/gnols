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
	go install golang.org/x/tools/gopls@v0.15.3
	# can't use @latest or @master because that fetches a nightly build where
	# the `gno transpile -gobuild .` fails because there's no go.mod in the repo.
	# This shouldn't be the case as long as a list of files is given to the
	# `go build` command rather than a path.
	go install github.com/gnolang/gno/gnovm/cmd/gno@6ec4bb80

build:
	GOOS=$(os) GOARCH=$(arch) go build ${LDFLAGS} -o bin/$(exe) ./cmd/gnols

gob:
	go run cmd/gen/main.go --root-dir "/Users/jdkato/Documents/Code/Gno/gno" 

json:
	go run cmd/gen/main.go --root-dir "/Users/jdkato/Documents/Code/Gno/gno" --format json
