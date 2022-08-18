
all: check static

# info embedded to the binary
PROJECT=github.com/intel/fakedev-exporter
GOVERSION=$(shell go version | sed 's/^[^0-9]*//' | cut -d' ' -f1)
BUILDUSER=$(shell git config user.email)
BUILDDATE=$(shell date "+%Y%m%d-%T")
VERSION=$(shell git describe --tags --abbrev=0)
COMMIT=$(shell git rev-parse --short HEAD)
BRANCH=$(shell git branch --show-current)

# build static PIE version
BUILDMODE=-buildmode=pie
EXTLDFLAGS=-static-pie

# build static version
#BUILDMODE=
#EXTLDFLAGS=-static

GOTAGS=osusergo,netgo,static

LDFLAGS = \
-s -w -linkmode external -extldflags $(EXTLDFLAGS) \
-X $(PROJECT)/version.GoVersion=$(GOVERSION) \
-X $(PROJECT)/version.BuildUser=$(BUILDUSER) \
-X $(PROJECT)/version.BuildDate=$(BUILDDATE) \
-X $(PROJECT)/version.Version=$(VERSION) \
-X $(PROJECT)/version.Revision=$(COMMIT) \
-X $(PROJECT)/version.Branch=$(BRANCH)

EXPORTER_SRC = $(wildcard cmd/fakedev-exporter/*.go)
WORKLOAD_SRC = $(wildcard cmd/fakedev-workload/*.go)


# static binaries
#
# packages: golang (v1.18 or newer)
static: fakedev-exporter fakedev-workload

fakedev-exporter: $(EXPORTER_SRC)
	go build $(BUILDMODE) -tags $(GOTAGS) -ldflags "$(LDFLAGS)" -o $@ $^

fakedev-workload: $(WORKLOAD_SRC)
	go build $(BUILDMODE) -tags $(GOTAGS) -ldflags "$(LDFLAGS)" -o $@ $^


# memory analysis binary versions
#
# packages: clang
msan: fakedev-workload-msan fakedev-exporter-msan

# "-msan" requires "CC=clang", dynamic
fakedev-workload-msan: $(WORKLOAD_SRC)
	CC=clang go build -msan $(BUILDMODE) -o $@ $^

fakedev-exporter-msan: $(EXPORTER_SRC)
	CC=clang go build -msan $(BUILDMODE) -o $@ $^


# data race detection binaries
race: fakedev-exporter-race

# race detector does not work with PIE
fakedev-exporter-race: $(EXPORTER_SRC)
	go build -race -ldflags "-linkmode external -extldflags -static" \
	   -tags $(GOTAGS) -o $@ $^


# packages: golang-x-lint (Fedora)
# or: go get -u golang.org/x/lint/golint
check:
	go fmt ./...
	golint ./...
	go vet ./...

mod:
	go mod tidy


clean:
	rm -rf fakedev-exporter fakedev-exporter-* \
	       fakedev-workload fakedev-workload-*

goclean: clean
	go clean --modcache

.PHONY: static msan race check mod clean goclean
