.PHONY: all build clean install test

BINDIR := bin
CMDS := $(notdir $(wildcard cmd/*))

all: build

build: $(CMDS)

$(CMDS):
	go build -o $(BINDIR)/$@ ./cmd/$@

clean:
	rm -f $(BINDIR)/*

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
