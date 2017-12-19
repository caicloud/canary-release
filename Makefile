# Copyright 2017 The Caicloud Authors.
#
# Old-skool build tools.
#
# Commonly used targets (see each target for more information):
#   all: Build code.
#   test: Run tests.
#   clean: Clean up.
# 
# see docs/Makefile.md for more information.

# It's necessary to set this because some environments don't link sh -> bash.
SHELL := /bin/bash

# We don't need make's built-in rules.
MAKEFLAGS += --no-builtin-rules
.SUFFIXES:

# =========================================================
# Tweak the variables based on your project.
# =========================================================

# Constants used throughout.
.EXPORT_ALL_VARIABLES:
# This controls the verbosity of the build.  Higher numbers mean more output.
VERBOSE ?= 1

# If true, built on local. Otherwise, built in docker.
LOCAL_BUILD ?= true
# Golang on-build docker image.
GO_ONBUILD_IMAGE := cargo.caicloudprivatetest.com/caicloud/golang:1.9.2-alpine3.6
# Building for these platforms.
GO_BUILD_PLATFORMS ?= linux/amd64 darwin/amd64
# Pre-defined all directory names of targets for go build. 
GO_BUILD_TARGETS := cmd/controller cmd/nginx-proxy
# Targets using CGO_ENABLED=0. It is a single word without dir prefix.
GO_STATIC_LIBRARIES := 
# Skip go unittest under the following dir.
GO_TEST_EXCEPTIONS := 

# Pre-defined all directories containing Dockerfiles for building containers.
DOCKER_BUILD_TARGETS := build/controller build/nginx-proxy
# Container registries.
DOCKER_REGISTRIES := cargo.caicloudprivatetest.com/caicloud
# Force pushing to override images in remote registries
DOCKER_FORCE_PUSH ?= true
# Container image prefix and suffix added to targets.
# The final built images are:
#   $[REGISTRY]/$[IMAGE_PREFIX]$[TARGET]$[IMAGE_SUFFIX]:$[VERSION]
#   $[REGISTRY] is an item from $[DOCKER_REGISTRIES], $[TARGET] is the basename from $[DOCKER_BUILD_TARGETS[@]].
DOCKER_IMAGE_PREFIX := $(strip canary-)
DOCKER_IMAGE_SUFFIX := $(strip )

define ALL_HELP_INFO
# Build code.
# make all == make build
#
# Args:
#   WHAT: Directory names to build.  If any of these directories has a 'main'
#     package, the build will produce executable files under bin/.
#     If not specified, "everything" will be built.
#   GOFLAGS: Extra flags to pass to 'go' when building.
#   GOLDFLAGS: Extra linking flags passed to 'go' when building.
#   GOGCFLAGS: Additional go compile flags passed to 'go' when building.
#
# Example:
#   make
#   make all or make build
#   make build WHAT=cmd/server GOFLAGS=-v
#   make all GOGCFLAGS="-N -l"
#     Note: Use the -N -l options to disable compiler optimizations an inlining.
#           Using these build options allows you to subsequently use source
#           debugging tools like delve.
endef
.PHONY: all build
ifeq ($(PRINT_HELP),y)
all build:
	@echo "$$ALL_HELP_INFO"
else
all build:
	hack/make-rules/build.sh $(WHAT)
endif 

define UNITTEST_HELP_INFO
# Run uniitest 
#
# Args:
#   GOFLAGS: Extra flags to pass to 'go' when building.
#   GOLDFLAGS: Extra linking flags passed to 'go' when building.
#   GOGCFLAGS: Additional go compile flags passed to 'go' when building.
#
# Example:
#   make
#   make unittest
#   make unittest WHAT=cmd/server GOFLAGS=-v
#   make unittest GOGCFLAGS="-N -l"
#     Note: Use the -N -l options to disable compiler optimizations an inlining.
#           Using these build options allows you to subsequently use source
#           debugging tools like delve.
endef
.PHONY: unittest
ifeq ($(PRINT_HELP),y)
unittest:
	@echo "$$UNITTEST_HELP_INFO"
else
unittest:
	hack/make-rules/unittest.sh
endif 

define BUILD_LOCAL_HELP_INFO
# Build code on local.
#
# Args:
#   WHAT: Directory names to build.  If any of these directories has a 'main'
#     package, the build will produce executable files under bin/.
#     If not specified, "everything" will be built.
#   GOFLAGS: Extra flags to pass to 'go' when building.
#   GOLDFLAGS: Extra linking flags passed to 'go' when building.
#   GOGCFLAGS: Additional go compile flags passed to 'go' when building.
#
# Example:
#   make
#   make build-local
#   make build-local WHAT=cmd/server GOFLAGS=-v
#   make build-local GOGCFLAGS="-N -l"
#     Note: Use the -N -l options to disable compiler optimizations an inlining.
#           Using these build options allows you to subsequently use source
#           debugging tools like delve.
endef
.PHONY: build-local
ifeq ($(PRINT_HELP),y)
build-local:
	@echo "$$BUILD_LOCAL_HELP_INFO"
else
build-local:
	LOCAL_BUILD=true hack/make-rules/build.sh $(WHAT) 
endif 

define BUILD_IN_CONTAINER_HELP_INFO
# Build code in container.
#
# Args:
#   WHAT: Directory names to build.  If any of these directories has a 'main'
#     package, the build will produce executable files under bin/.
#     If not specified, "everything" will be built.
#   GOFLAGS: Extra flags to pass to 'go' when building.
#   GOLDFLAGS: Extra linking flags passed to 'go' when building.
#   GOGCFLAGS: Additional go compile flags passed to 'go' when building.
#
# Example:
#   make
#   make build-in-container
#   make build-in-container WHAT=cmd/server GOFLAGS=-v
#   make build-in-container GOGCFLAGS="-N -l"
#     Note: Use the -N -l options to disable compiler optimizations an inlining.
#           Using these build options allows you to subsequently use source
#           debugging tools like delve.
endef
.PHONY: build-in-container
ifeq ($(PRINT_HELP),y)
build-in-container:
	@echo "$$BUILD_LINUX_HELP_INFO"
else
build-in-container:
	LOCAL_BUILD=false hack/make-rules/build.sh $(WHAT) 
endif 

define CONTAINER_HELP_INFO
# Build docker image.
#
# Args:
#   WHAT: Directories containing Dockerfile.
#
# Example:
#   make container
#   make container WAHT=build/server
endef
.PHONY: container 
ifeq ($(PRINT_HELP),y)
container:
	@echo "$$CONTAINER_HELP_INFO"
else
container:
	PRJ_DOCKER_BUILD=1 hack/make-rules/docker.sh $(WHAT)
endif 

define PUSH_HELP_INFO
# Push docker image.
# You should run make container before push
#
# Args:
#   WHAT: Directory names containing Dockerfile.
#
# Example:
#   make push
#   make push WAHT=build/server
endef
.PHONY: push 
ifeq ($(PRINT_HELP),y)
push:
	@echo "$$PUSH_HELP_INFO"
else
push:
	PRJ_DOCKER_PUSH=1 hack/make-rules/docker.sh $(WHAT)
endif 


define CLEAN_HELP_INFO
# Remove all build artifacts.
#
# Example:
#   make clean
#
endef
.PHONY: clean
ifeq ($(PRINT_HELP),y)
clean:
	@echo "$$CLEAN_HELP_INFO"
else
clean:
	hack/make-rules/clean.sh
endif
