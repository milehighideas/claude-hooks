# Claude Hooks — local development and CI simulation

# Run all tests
test:
    go test ./...

# Build for current platform
build:
    make build

# Cross-compile for all platforms (mirrors release.yml)
build-all:
    make build-all

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

# Clean build artifacts
clean:
    make clean

# Install binaries to /usr/local/bin (current platform only)
install: build
    make install
