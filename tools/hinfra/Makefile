BINARY := hinfra
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build install test clean

build:
	go build -o bin/$(BINARY) ./cmd/hinfra

install:
	go build -o $(INSTALL_DIR)/$(BINARY) ./cmd/hinfra
	@echo "Instalado em $(INSTALL_DIR)/$(BINARY)"
	@echo "Registre MCP: hinfra mcp install"

test:
	go test ./...

clean:
	rm -rf bin/
