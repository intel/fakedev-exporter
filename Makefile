# fakedev-exporter

help:
	@echo -e "\nTargets:\n$$(grep '^[a-z]*:' Makefile | sed -e 's/^/- /' -e 's/:$$//')\n"

build: gocheck static

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
INVALID_SRC  = $(wildcard cmd/invalid-workload/*.go)


# static binaries
#
# packages: golang
static: fakedev-exporter fakedev-workload invalid-workload

fakedev-exporter: $(EXPORTER_SRC)
	go build $(BUILDMODE) -tags $(GOTAGS) -ldflags "$(LDFLAGS)" -o $@ $^

fakedev-workload: $(WORKLOAD_SRC)
	go build $(BUILDMODE) -tags $(GOTAGS) -ldflags "$(LDFLAGS)" -o $@ $^

invalid-workload: $(INVALID_SRC)
	go build $(BUILDMODE) -tags $(GOTAGS) -ldflags "$(LDFLAGS)" -o $@ $^


# data race detection binaries

race: fakedev-exporter-race

# race detector does not work with PIE
fakedev-exporter-race: $(EXPORTER_SRC)
	go build -race -ldflags "-linkmode external -extldflags -static" \
	   -tags $(GOTAGS) -o $@ $^


BINDIR ?= $(shell pwd)

# packages: wget psmisc diffutils
test-race: fakedev-exporter-race fakedev-workload invalid-workload
	./test-exporter.sh \
	  $(BINDIR)/fakedev-exporter-race \
	  $(BINDIR)/fakedev-workload \
	  $(BINDIR)/invalid-workload

test: test-race
	./test-deployment.sh


# packages: golang-x-lint (Fedora)
# or: go get -u golang.org/x/lint/golint
gocheck:
	go fmt ./...
	golint ./...
	go vet ./...

mod:
	go mod tidy


# checks for auxiliary / test scripts

# packages: shellcheck
shellcheck:
	find . -name '*.sh' | xargs shellcheck

check: gocheck shellcheck


clean:
	rm -rf fakedev-exporter fakedev-exporter-* \
	       fakedev-workload fakedev-workload-* \
	       invalid-workload

goclean: clean
	go clean --modcache

.PHONY: help build static race test-race test \
	gocheck shellcheck check \
	mod clean goclean
