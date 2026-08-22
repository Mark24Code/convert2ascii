GO ?= go

.PHONY: build test clean

build:
	mkdir -p bin
	$(GO) build -o bin/image2ascii ./cmd/image2ascii
	$(GO) build -o bin/video2ascii ./cmd/video2ascii

test:
	$(GO) test ./...

clean:
	rm -rf bin
