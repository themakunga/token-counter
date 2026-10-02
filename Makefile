BIN     := token-counter
PREFIX  ?= /usr/local/bin
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build run install uninstall update clean dist

build:
	go build -ldflags="$(LDFLAGS)" -o $(BIN) .

run:
	go run .

install: build
	install -m 755 $(BIN) $(PREFIX)/$(BIN)

uninstall:
	rm -f $(PREFIX)/$(BIN)

update:
	git pull --ff-only
	$(MAKE) install

clean:
	rm -f $(BIN)
	rm -rf dist/

dist:
	mkdir -p dist
	for GOOS in darwin linux; do \
		for GOARCH in amd64 arm64; do \
			GOOS=$$GOOS GOARCH=$$GOARCH go build \
				-ldflags="$(LDFLAGS)" \
				-o "dist/$(BIN)-$$GOOS-$$GOARCH" .; \
		done; \
	done
	cd dist && sha256sum * > checksums.txt
