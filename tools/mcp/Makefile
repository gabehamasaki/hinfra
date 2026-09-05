BINARY := infra-mcp
INSTALL_DIR := $(HOME)/.local/bin

.PHONY: build install test clean

build:
	go build -o bin/$(BINARY) ./cmd/infra-mcp

install:
	go build -o $(INSTALL_DIR)/$(BINARY) ./cmd/infra-mcp
	@echo "Instalado em $(INSTALL_DIR)/$(BINARY)"
	@echo "Registre com: claude mcp add --scope user infra -- $(INSTALL_DIR)/$(BINARY)"

test:
	go test ./...

clean:
	rm -rf bin/
