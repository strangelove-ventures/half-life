#!/usr/bin/make -f

BRANCH := $(shell git rev-parse --abbrev-ref HEAD)
COMMIT := $(shell git log -1 --format='%H')

# don't override user values
ifeq (,$(VERSION))
  VERSION := $(shell git describe --tags)
  # if VERSION is empty, then populate it with branch's name and raw commit hash
  ifeq (,$(VERSION))
    VERSION := $(BRANCH)-$(COMMIT)
  endif
endif

PACKAGES_SIMTEST=$(shell go list ./... | grep '/simulation')
DOCKER := $(shell which docker)
BUILDDIR ?= $(CURDIR)/build
export GO111MODULE = on

build_tags += $(BUILD_TAGS)
build_tags := $(strip $(build_tags))
ldflags += $(LDFLAGS)
ldflags := $(strip $(ldflags))

# process linker flags
ldflags = -X "github.com/cosmos/cosmos-sdk/version.BuildTags=$(build_tags_comma_sep)"
ldflags += -w -s

BUILD_FLAGS := -tags "$(build_tags)" -ldflags '$(ldflags)'

all: install

install: go.sum
	go install .

build:
	go build -o bin/halflife .

clean:
	rm -rf build

build-half-life-docker:
	docker build -t ghcr.io/strangelove-ventures/half-life/halflife:$(VERSION) -f ./Dockerfile .
	docker push ghcr.io/strangelove-ventures/half-life/halflife:$(VERSION)