BIN := token-counter
PREFIX ?= /usr/local/bin

.PHONY: build run install uninstall clean

build:
	go build -o $(BIN) .

run:
	go run .

install: build
	install -m 755 $(BIN) $(PREFIX)/$(BIN)

uninstall:
	rm -f $(PREFIX)/$(BIN)

clean:
	rm -f $(BIN)
