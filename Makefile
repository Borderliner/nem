# CGO_ENABLED=0 is not optional. Go enables cgo by default when a C compiler is
# present, and a Lip Gloss dependency pulls in os/user, which links libc for NSS
# lookups - so a plain `go build` produces a dynamically linked binary. With it
# off, nem is fully static: no libc, no glibc version skew, runs on musl.
export CGO_ENABLED = 0

VERSION ?= dev
LDFLAGS  = -s -w -X main.version=$(VERSION)

.PHONY: build install test lint clean

# -s -w drop the symbol table and DWARF: 7.4MB -> 5.1MB.
build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o nem ./cmd/nem

install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/nem

test:
	go test ./... -race -count=1

lint:
	gofmt -l .
	go vet ./...

clean:
	rm -f nem
