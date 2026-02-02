.PHONY: all build clean install test build-all build-darwin build-linux build-windows

BINDIR := bin
CMDS := $(notdir $(wildcard cmd/*))

all: build

# Build for current platform (default)
build: $(CMDS)

$(CMDS):
	go build -o $(BINDIR)/$@ ./cmd/$@

# Cross-compile for all platforms
build-all: build-darwin build-linux build-windows

build-darwin: $(CMDS:%=build-darwin-%)
build-darwin-%:
	GOOS=darwin GOARCH=arm64 go build -o $(BINDIR)/darwin-arm64/$* ./cmd/$*

build-linux: $(CMDS:%=build-linux-%)
build-linux-%:
	GOOS=linux GOARCH=amd64 go build -o $(BINDIR)/linux-amd64/$* ./cmd/$*

build-windows: $(CMDS:%=build-windows-%)
build-windows-%:
	GOOS=windows GOARCH=amd64 go build -o $(BINDIR)/windows-amd64/$*.exe ./cmd/$*

clean:
	rm -rf $(BINDIR)/*

install: build
	@echo "Installing binaries to /usr/local/bin..."
	@for bin in $(BINDIR)/*; do \
		cp $$bin /usr/local/bin/; \
	done

test:
	go test ./...

# Build a specific command
build-%:
	go build -o $(BINDIR)/$* ./cmd/$*
