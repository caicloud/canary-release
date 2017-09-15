# Copyright 2017 The Caicloud Authors.
#
# The old school Makefile, following are required targets. The Makefile is written
# to allow building multiple binaries. You are free to add more targets or change
# existing implementations, as long as the semantics are preserved.
#
#   make        - default to 'build' target
#   make lint   - code analysis
#   make test   - run unit test (or plus integration test)
#   make build        - alias to build-local target
#   make build-local  - build local binary targets
#   make build-linux  - build linux binary targets
#   make container    - build containers
#   make push    - push containers
#   make clean   - clean up targets
#
# Not included but recommended targets:
#   make e2e-test
#
# The makefile is also responsible to populate project version information.



# Current version of the project.
VERSION ?= v0.1.0

#
# These variables should not need tweaking.
#
 
# A list of all packages.
PKGS := $(shell go list ./... | grep -v /vendor | grep -v /tests)
 
# Project main package location (can be multiple ones).
CMD_DIR := ./cmd
 
# Deployment direcotory.
BUILD_DIR := ./build

# Git commit sha.
COMMIT := $(shell git rev-parse --short HEAD)

# Golang standard bin directory.
BIN_DIR := $(GOPATH)/bin
GOMETALINTER := $(BIN_DIR)/gometalinter

#
# Tweak the variables based on your project.
#

# this pkg 
PKG := github.com/caicloud/canary-release

# This repo's root import path (under GOPATH).
ROOT := $(PKG)/proxies/nginx
 
# Target binaries. You can build multiple binaries for a single project.
TARGETS := proxy
 
# Container image prefix and suffix added to targets.
# The final built images are:
#   $[REGISTRY]/$[IMAGE_PREFIX]$[TARGET]$[IMAGE_SUFFIX]:$[VERSION]
# $[REGISTRY] is an item from $[REGISTRIES], $[TARGET] is an item from $[TARGETS].
IMAGE_PREFIX ?= $(strip canary-)
IMAGE_SUFFIX ?= $(strip -nginx)
 
# Container registries.
REGISTRIES ?= cargo.caicloudprivatetest.com/caicloud

ARCH ?= amd64
GOARCH = $(ARCH)
DUMB_ARCH = $(ARCH)
QEMUVERSION=v2.7.0

IMAGE = $(IMAGE_PREFIX)$(TARGETS)$(IMAGE_SUFFIX)

BASEIMAGE?=gcr.io/google_containers/nginx-slim-$(ARCH):0.21

TEMP_DIR := $(shell mktemp -d)

# Project output directory.
OUTPUT_DIR := $(TEMP_DIR)/build/rootfs

#
# Define all targets. At least the following commands are required:
#
 
# All targets.
.PHONY: lint test build container push

build: build-local

lint: $(GOMETALINTER)
	gometalinter ./... --vendor
 
$(GOMETALINTER):
	go get -u github.com/alecthomas/gometalinter
	gometalinter --install &> /dev/null
 
test:
	 @go test $(PKGS)

build-local: 
	go build -i -v -o $(OUTPUT_DIR)/$(TARGETS) \
	-ldflags "-s -w -X $(PKG)/pkg/version.RELEASE=$(VERSION) -X $(PKG)/pkg/version.COMMIT=$(COMMIT) -X $(PKG)/pkg/version.REPO=$(ROOT)" \
	$(ROOT)/cmd

build-linux: 
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -i -v -o $(OUTPUT_DIR)/$(TARGETS) \
	-installsuffix cgo \
	-ldflags "-s -w -X $(PKG)/pkg/version.RELEASE=$(VERSION) -X $(PKG)/pkg/version.COMMIT=$(COMMIT) -X $(PKG)/pkg/version.REPO=$(ROOT)" \
	$(ROOT)/cmd

container: build-linux
	cp -r ./* $(TEMP_DIR)
	sed -i 's|BASEIMAGE|$(BASEIMAGE)|g' $(OUTPUT_DIR)/Dockerfile
	sed -i "s|QEMUARCH|$(QEMUARCH)|g" $(OUTPUT_DIR)/Dockerfile
	sed -i "s|DUMB_ARCH|$(DUMB_ARCH)|g" $(OUTPUT_DIR)/Dockerfile

	test -e $(OUTPUT_DIR)/$(TARGETS)

	@docker build -t $(IMAGE):$(VERSION) $(OUTPUT_DIR)

	@for registry in $(REGISTRIES); do \
		docker tag $(IMAGE):$(VERSION) $${registry}/$(IMAGE):$(VERSION); \
	done

	@docker rmi $(IMAGE):$(VERSION)

push: container
	@for registry in $(REGISTRIES); do \
		docker push $${registry}/$(IMAGE):$(VERSION); \
	done

clean:
	@for registry in $(REGISTRIES); do \
		docker rmi -f $${registry}/$(IMAGE):$(VERSION) || true; \
	done
