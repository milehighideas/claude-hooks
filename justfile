bindir := "bin"

# List available recipes
default:
    @just --list

# Build all binaries
build: auto-convex-gen block-destructive-commands block-generated-files block-infrastructure block-lint-workarounds changelog-add convex-gen docs-tracker enforce-tests-on-commit format-on-save markdown-formatter pre-commit smart-lint smart-test track-edited-files validate-frontend-structure validate-srp validate-test-files

# Individual binaries
auto-convex-gen:
    go build -o {{bindir}}/auto-convex-gen ./cmd/auto-convex-gen

block-destructive-commands:
    go build -o {{bindir}}/block-destructive-commands ./cmd/block-destructive-commands

block-generated-files:
    go build -o {{bindir}}/block-generated-files ./cmd/block-generated-files

block-infrastructure:
    go build -o {{bindir}}/block-infrastructure ./cmd/block-infrastructure

block-lint-workarounds:
    go build -o {{bindir}}/block-lint-workarounds ./cmd/block-lint-workarounds

changelog-add:
    go build -o {{bindir}}/changelog-add ./cmd/changelog-add

convex-gen:
    go build -o {{bindir}}/convex-gen ./cmd/convex-gen

docs-tracker:
    go build -o {{bindir}}/docs-tracker ./cmd/docs-tracker

enforce-tests-on-commit:
    go build -o {{bindir}}/enforce-tests-on-commit ./cmd/enforce-tests-on-commit

format-on-save:
    go build -o {{bindir}}/format-on-save ./cmd/format-on-save

markdown-formatter:
    go build -o {{bindir}}/markdown-formatter ./cmd/markdown-formatter

pre-commit:
    go build -o {{bindir}}/pre-commit ./cmd/pre-commit

smart-lint:
    go build -o {{bindir}}/smart-lint ./cmd/smart-lint

smart-test:
    go build -o {{bindir}}/smart-test ./cmd/smart-test

track-edited-files:
    go build -o {{bindir}}/track-edited-files ./cmd/track-edited-files

validate-frontend-structure:
    go build -o {{bindir}}/validate-frontend-structure ./cmd/validate-frontend-structure

validate-srp:
    go build -o {{bindir}}/validate-srp ./cmd/validate-srp

validate-test-files:
    go build -o {{bindir}}/validate-test-files ./cmd/validate-test-files

# Run all tests
test:
    go test ./...

# Clean all binaries
clean:
    rm -rf {{bindir}}/*

# Install binaries to /usr/local/bin
install: build
    #!/usr/bin/env bash
    echo "Installing binaries to /usr/local/bin..."
    for bin in {{bindir}}/*; do
        [ -f "$bin" ] && cp "$bin" /usr/local/bin/
    done

# Cross-compile for all platforms
build-all: build-darwin build-linux build-windows

# Build for macOS (Apple Silicon)
build-darwin:
    #!/usr/bin/env bash
    mkdir -p {{bindir}}/darwin-arm64
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        GOOS=darwin GOARCH=arm64 go build -o {{bindir}}/darwin-arm64/$name ./cmd/$name
    done

# Build for Linux (amd64)
build-linux:
    #!/usr/bin/env bash
    mkdir -p {{bindir}}/linux-amd64
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        GOOS=linux GOARCH=amd64 go build -o {{bindir}}/linux-amd64/$name ./cmd/$name
    done

# Build for Windows (amd64)
build-windows:
    #!/usr/bin/env bash
    mkdir -p {{bindir}}/windows-amd64
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        GOOS=windows GOARCH=amd64 go build -o {{bindir}}/windows-amd64/$name.exe ./cmd/$name
    done

# Package archives (mirrors release.yml packaging step)
package: build-all
    cd bin && \
    for platform in darwin-arm64 linux-amd64 windows-amd64; do \
        tar -czf "claude-hooks-${platform}.tar.gz" -C "$platform" . ; \
    done
    @echo "Archives created in bin/"
    @ls -lh bin/*.tar.gz

# Run the full CI pipeline locally (test + build-all + package)
ci: test package
    @echo ""
    @echo "CI simulation passed — all platforms built and packaged."
