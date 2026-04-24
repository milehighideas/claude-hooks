bindir := "bin"

# List available recipes
default:
    @just --list

# Build all binaries
build: check-workspace auto-convex-gen auto-tiers-gen block-destructive-commands block-generated-files block-infrastructure block-lint-workarounds block-redundant-createdat changelog-add convex-gen docs-tracker enforce-tests-on-commit format-on-save markdown-formatter pre-commit smart-lint smart-test track-edited-files validate-frontend-structure validate-srp validate-test-files

# Fail if any executable exists at the repo root with the same name as a cmd/*/ subdir.
# These get created when someone runs `go build ./cmd/<name>` from the repo root without -o,
# which pollutes the workspace. Legit builds go to bin/ via `just <name>`.
check-workspace:
    #!/usr/bin/env bash
    set -euo pipefail
    shopt -s nullglob
    stray=()
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        if [ -f "$name" ]; then
            stray+=("$name")
        fi
    done
    if [ ${#stray[@]} -gt 0 ]; then
        echo "Error: found stray binaries at repo root:" >&2
        for name in "${stray[@]}"; do
            echo "  - $name" >&2
        done
        echo "" >&2
        echo "These were probably built with 'go build ./cmd/<name>' from the repo root." >&2
        echo "Use 'just <name>' or 'just build' — they write to {{bindir}}/ where they belong." >&2
        echo "" >&2
        echo "Fix: just clean" >&2
        exit 1
    fi

# Individual binaries
auto-convex-gen:
    go build -o {{bindir}}/auto-convex-gen ./cmd/auto-convex-gen

auto-tiers-gen:
    go build -o {{bindir}}/auto-tiers-gen ./cmd/auto-tiers-gen

block-destructive-commands:
    go build -o {{bindir}}/block-destructive-commands ./cmd/block-destructive-commands

block-generated-files:
    go build -o {{bindir}}/block-generated-files ./cmd/block-generated-files

block-infrastructure:
    go build -o {{bindir}}/block-infrastructure ./cmd/block-infrastructure

block-lint-workarounds:
    go build -o {{bindir}}/block-lint-workarounds ./cmd/block-lint-workarounds

block-redundant-createdat:
    go build -o {{bindir}}/block-redundant-createdat ./cmd/block-redundant-createdat

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

# Clean all binaries (bin/ and any stray root binaries from misguided `go build`s)
clean:
    #!/usr/bin/env bash
    set -euo pipefail
    shopt -s nullglob
    rm -rf {{bindir}}/*
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        if [ -f "$name" ]; then
            echo "Removing stray root binary: $name"
            rm "$name"
        fi
    done

# Install binaries to /usr/local/bin
install: build
    #!/usr/bin/env bash
    echo "Installing binaries to /usr/local/bin..."
    for bin in {{bindir}}/*; do
        [ -f "$bin" ] && cp "$bin" /usr/local/bin/
    done

# Cross-compile for all platforms.
#
# NOTE: validate-test-files and pre-commit depend on tree-sitter (CGO) for
# AST-based stub detection. Cross-compiling CGO code requires platform-
# specific C toolchains on the build host:
#
#   macOS -> Linux:    brew install FiloSottile/musl-cross/musl-cross
#   macOS -> Windows:  brew install mingw-w64
#   macOS -> arm64:    brew install aarch64-unknown-linux-gnu  (or zig cc)
#
# If those aren't installed the respective platform build will fail with
# "cgo: C compiler not available". Native builds (`just <cmdname>`) work
# out of the box on the host platform.
build-all: check-workspace build-darwin build-linux build-linux-arm64 build-windows

# Build for macOS (Apple Silicon)
build-darwin:
    #!/usr/bin/env bash
    mkdir -p {{bindir}}/darwin-arm64
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 go build -o {{bindir}}/darwin-arm64/$name ./cmd/$name
    done

# Build for Linux (amd64) — requires musl-cross or equivalent for CGO
build-linux:
    #!/usr/bin/env bash
    mkdir -p {{bindir}}/linux-amd64
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        CGO_ENABLED=1 CC=${CC_LINUX_AMD64:-x86_64-linux-musl-gcc} \
          GOOS=linux GOARCH=amd64 go build -o {{bindir}}/linux-amd64/$name ./cmd/$name
    done

# Build for Linux (arm64 — Docker on Apple Silicon) — requires aarch64 cross-compiler for CGO
build-linux-arm64:
    #!/usr/bin/env bash
    mkdir -p {{bindir}}/linux-arm64
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        CGO_ENABLED=1 CC=${CC_LINUX_ARM64:-aarch64-linux-musl-gcc} \
          GOOS=linux GOARCH=arm64 go build -o {{bindir}}/linux-arm64/$name ./cmd/$name
    done

# Build for Windows (amd64) — requires mingw-w64 for CGO
build-windows:
    #!/usr/bin/env bash
    mkdir -p {{bindir}}/windows-amd64
    for cmd in cmd/*/; do
        name=$(basename "$cmd")
        CGO_ENABLED=1 CC=${CC_WINDOWS_AMD64:-x86_64-w64-mingw32-gcc} \
          GOOS=windows GOARCH=amd64 go build -o {{bindir}}/windows-amd64/$name.exe ./cmd/$name
    done

# Package archives (mirrors release.yml packaging step)
package: build-all
    cd bin && \
    for platform in darwin-arm64 linux-amd64 linux-arm64 windows-amd64; do \
        tar -czf "claude-hooks-${platform}.tar.gz" -C "$platform" . ; \
    done
    @echo "Archives created in bin/"
    @ls -lh bin/*.tar.gz

# Run the full CI pipeline locally (test + build-all + package)
ci: test package
    @echo ""
    @echo "CI simulation passed — all platforms built and packaged."
