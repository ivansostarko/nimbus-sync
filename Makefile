BINARY  := nimbus
PKG     := ./cmd/nimbus
LDFLAGS := -s -w

.PHONY: build install test lint clean release

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) $(PKG)

install: build
	install -m 0755 bin/$(BINARY) /usr/local/bin/$(BINARY)

test:
	go test ./...

lint:
	go vet ./...

clean:
	rm -rf bin dist

# Cross-compile release binaries for common Linux targets.
release:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64 $(PKG)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64 $(PKG)
	cd dist && sha256sum * > SHA256SUMS
