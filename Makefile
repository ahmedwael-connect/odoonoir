BINARY := odoonoir
VERSION ?= 0.8.0
LDFLAGS := -ldflags "-X github.com/ahmed/odoonoir/internal/cli.Version=$(VERSION)"
PREFIX ?= $(HOME)/.local
INSTALL_DIR := $(PREFIX)/bin

.PHONY: build install test vet clean

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/odoonoir

install: build
	install -d $(INSTALL_DIR)
	install -m 0755 bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@if ! echo "$$PATH" | tr ':' '\n' | grep -qx "$(INSTALL_DIR)"; then \
		echo "note: $(INSTALL_DIR) is not on your PATH."; \
		echo "add it with:  echo 'export PATH=\$$PATH:$(INSTALL_DIR)' >> ~/.bashrc"; \
	fi

test:
	go test ./...

vet:
	go vet ./...

clean:
	rm -rf bin