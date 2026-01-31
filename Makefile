# Copyright (c) 2019 The BFE Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# init project path
WORKROOT := $(shell pwd)
OUTDIR   := $(WORKROOT)/output
OS		 := $(shell go env GOOS)

# init environment variables
export PATH        := $(shell go env GOPATH)/bin:$(PATH)
export GO111MODULE := on

# init command params
GO           := go
GOBUILD      := $(GO) build
GOTEST       := $(GO) test
GOVET        := $(GO) vet
GOGET        := $(GO) get
GOGEN        := $(GO) generate
GOCLEAN      := $(GO) clean
GOINSTALL    := $(GO) install
GOFLAGS      := -race
STATICCHECK  := staticcheck
LICENSEEYE   := license-eye
PIP          := pip3
PIPINSTALL   := $(PIP) install

# init arch
ARCH := $(shell getconf LONG_BIT)
ifeq ($(ARCH),64)
	GOTEST += $(GOFLAGS)
endif

# init bfe version
BFE_VERSION ?= $(shell cat VERSION)
# init git commit id
GIT_COMMIT ?= $(shell git rev-parse HEAD)

# init bfe packages
BFE_PKGS := $(shell go list ./...)

# go install package
# $(1) package name
# $(2) package address
define INSTALL_PKG
	@echo installing $(1)
	$(GOINSTALL) $(2)
	@echo $(1) installed
endef

define PIP_INSTALL_PKG
	@echo installing $(1)
	$(PIPINSTALL) $(1)
	@echo $(1) installed
endef

# make, make all
all: prepare compile package

# make, make strip
strip: prepare compile-strip package

# make prepare, download dependencies
prepare: prepare-dep prepare-gen
prepare-dep:
	$(call INSTALL_PKG, goyacc, golang.org/x/tools/cmd/goyacc@latest)
prepare-gen:
	cd "bfe_basic/condition/parser" && $(GOGEN)

# make compile, go build
compile: test build
build:
ifeq ($(OS),darwin)
	$(GOBUILD) -ldflags "-X main.version=$(BFE_VERSION) -X main.commit=$(GIT_COMMIT)"
else
	$(GOBUILD) -ldflags "-X main.version=$(BFE_VERSION) -X main.commit=$(GIT_COMMIT) -extldflags=-static"
endif

# make compile-strip, go build without symbols and DWARFs
compile-strip: test build-strip
build-strip:
ifeq ($(OS),darwin)
	$(GOBUILD) -ldflags "-X main.version=$(BFE_VERSION) -X main.commit=$(GIT_COMMIT) -s -w"
else
	$(GOBUILD) -ldflags "-X main.version=$(BFE_VERSION) -X main.commit=$(GIT_COMMIT) -extldflags=-static -s -w"
endif

# make test, test your code
test: test-case vet-case
test-case:
	$(GOTEST) -cover ./...
vet-case:
	${GOVET} ./...

# make coverage for codecov
coverage:
	echo -n > coverage.txt
	for pkg in $(BFE_PKGS) ; do $(GOTEST) -coverprofile=profile.out -covermode=atomic $${pkg} && cat profile.out >> coverage.txt; done

# make package
package:
	mkdir -p $(OUTDIR)/bin
	mv bfe  $(OUTDIR)/bin
	cp -r conf $(OUTDIR)

# make deps
deps:
	$(call PIP_INSTALL_PKG, pre-commit)
	$(call INSTALL_PKG, goyacc, golang.org/x/tools/cmd/goyacc@latest)
	$(call INSTALL_PKG, staticcheck, honnef.co/go/tools/cmd/staticcheck)
	$(call INSTALL_PKG, license-eye, github.com/apache/skywalking-eyes/cmd/license-eye@latest)

# make precommit, enable autoupdate and install with hooks
precommit:
	pre-commit autoupdate
	pre-commit install --install-hooks

# make check
check:
	$(STATICCHECK) ./...

# make license-check, check code file's license declaration
license-check:
	$(LICENSEEYE) header check

# make license-fix, fix code file's license declaration
license-fix:
	$(LICENSEEYE) header fix


# Docker image build targets
BFE_IMAGE_NAME ?= bfe
# conf-agent version used in Docker image build.
# Default: 0.0.2
# Override example: make docker CONF_AGENT_VERSION=0.0.2
CONF_AGENT_VERSION ?= 0.0.2
NO_CACHE ?= false

# Optional cleanup controls
# - CLEAN_DANGLING=true will remove dangling images ("<none>:<none>") after build.
# - CLEAN_BUILDKIT_CACHE=true will prune build cache (can slow down next builds).
CLEAN_DANGLING ?= false
CLEAN_BUILDKIT_CACHE ?= false

# Optional buildx (multi-arch) settings
PLATFORMS ?= linux/amd64,linux/arm64
BUILDER_NAME ?= bfe-builder

# buildx helpers
# - make docker (local build) does NOT require buildx.
# - make docker-push (multi-arch push) requires buildx and will auto-init a builder.
buildx-check:
	@docker buildx version >/dev/null 2>&1 || ( \
		echo "Error: docker buildx is not available."; \
		echo "- If you use Docker Desktop: update/enable BuildKit/buildx."; \
		echo "- If you use docker-ce: install the buildx plugin."; \
		exit 1; \
	)

buildx-init: buildx-check
	@docker buildx inspect $(BUILDER_NAME) >/dev/null 2>&1 || docker buildx create --name $(BUILDER_NAME) --driver docker-container --use
	@docker buildx use $(BUILDER_NAME)
	@docker buildx inspect --bootstrap >/dev/null 2>&1 || true

# make docker: Build BFE docker images (prod + debug)
docker:
	@echo "Building BFE docker images (prod + debug)..."
	@NORM_BFE_VERSION=$$(echo "$(BFE_VERSION)" | sed 's/^v*/v/'); \
	NORM_CONF_VERSION=$$(echo "$(CONF_AGENT_VERSION)" | sed 's/^v*/v/'); \
	echo "BFE version: $$NORM_BFE_VERSION"; \
	echo "conf-agent version: $$NORM_CONF_VERSION"; \
	echo "Step 1/2: build prod image"; \
	docker build \
		$$(if [ "$(NO_CACHE)" = "true" ]; then echo "--no-cache"; fi) \
		--build-arg VARIANT=prod \
		--build-arg CONF_AGENT_VERSION=$$NORM_CONF_VERSION \
		-t $(BFE_IMAGE_NAME):$$NORM_BFE_VERSION \
		-t $(BFE_IMAGE_NAME):latest \
		-f Dockerfile \
		.; \
	echo "Step 2/2: build debug image"; \
	docker build \
		$$(if [ "$(NO_CACHE)" = "true" ]; then echo "--no-cache"; fi) \
		--build-arg VARIANT=debug \
		--build-arg CONF_AGENT_VERSION=$$NORM_CONF_VERSION \
		-t $(BFE_IMAGE_NAME):$$NORM_BFE_VERSION-debug \
		-f Dockerfile \
		.
	@$(MAKE) docker-prune

# docker-prune: optional post-build cleanup (safe-by-default)
docker-prune:
	@if [ "$(CLEAN_DANGLING)" = "true" ]; then \
		echo "Pruning dangling images (<none>)..."; \
		docker image prune -f; \
	fi
	@if [ "$(CLEAN_BUILDKIT_CACHE)" = "true" ]; then \
		echo "Pruning build cache (BuildKit)..."; \
		docker builder prune -f; \
	fi

# make docker-push: Build & push multi-arch images using buildx (REGISTRY is required)
# Usage: make docker-push REGISTRY=ghcr.io/your-org
docker-push:
	@if [ -z "$(REGISTRY)" ]; then \
		echo "Error: REGISTRY is required"; \
		echo "Usage: make docker-push REGISTRY=ghcr.io/your-org"; \
		exit 1; \
	fi
	@echo "Building and pushing multi-arch images via buildx..."
	@echo "Platforms: $(PLATFORMS)"
	@$(MAKE) buildx-init
	@NORM_BFE_VERSION=$$(echo "$(BFE_VERSION)" | sed 's/^v*/v/'); \
	NORM_CONF_VERSION=$$(echo "$(CONF_AGENT_VERSION)" | sed 's/^v*/v/'); \
	NO_CACHE_OPT=$$(if [ "$(NO_CACHE)" = "true" ]; then echo "--no-cache"; fi); \
	echo "BFE version: $$NORM_BFE_VERSION"; \
	echo "conf-agent version: $$NORM_CONF_VERSION"; \
	echo "Step 1/2: build+push prod (multi-arch)"; \
	docker buildx build \
		--platform $(PLATFORMS) \
		$$NO_CACHE_OPT \
		--build-arg VARIANT=prod \
		--build-arg CONF_AGENT_VERSION=$$NORM_CONF_VERSION \
		-t $(REGISTRY)/$(BFE_IMAGE_NAME):$$NORM_BFE_VERSION \
		-t $(REGISTRY)/$(BFE_IMAGE_NAME):latest \
		-f Dockerfile \
		--push \
		.; \
	echo "Step 2/2: build+push debug (multi-arch)"; \
	docker buildx build \
		--platform $(PLATFORMS) \
		$$NO_CACHE_OPT \
		--build-arg VARIANT=debug \
		--build-arg CONF_AGENT_VERSION=$$NORM_CONF_VERSION \
		-t $(REGISTRY)/$(BFE_IMAGE_NAME):$$NORM_BFE_VERSION-debug \
		-f Dockerfile \
		--push \
		.; \
	echo "Pushed multi-arch:"; \
	echo "  - $(REGISTRY)/$(BFE_IMAGE_NAME):$$NORM_BFE_VERSION"; \
	echo "  - $(REGISTRY)/$(BFE_IMAGE_NAME):$$NORM_BFE_VERSION-debug"; \
	echo "  - $(REGISTRY)/$(BFE_IMAGE_NAME):latest (prod)"
	@$(MAKE) docker-prune

# make clean
clean:
	$(GOCLEAN)
	rm -rf $(OUTDIR)
	rm -rf $(WORKROOT)/bfe
	rm -rf $(GOPATH)/pkg/linux_amd64

# avoid filename conflict and speed up build 
.PHONY: all prepare compile test package clean build docker docker-push docker-prune buildx-check buildx-init
