VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet fmt-check check install clean

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/lazytuck ./cmd/lazytuck

test:
	go test -race ./...

vet:
	go vet ./...

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

check: fmt-check vet test

install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/lazytuck

clean:
	rm -rf bin dist
