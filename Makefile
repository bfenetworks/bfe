# Copyright (c) 2019 Baidu, Inc.
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

# init environment variables
export GOPATH      := $(WORKROOT)/../../../../
export PATH        := $(GOPATH)/bin:$(PATH)
export GO111MODULE := on

# init command params
GO      := go
GOBUILD := $(GO) build
GOTEST  := $(GO) test
GOVET   := $(GO) vet
GOGET   := $(GO) get
GOGEN   := $(GO) generate

# init bfe version
BFE_VERSION ?= $(shell cat VERSION)

# make, make all
all: prepare compile package

# make prepare, download dependencies
prepare: prepare-dep prepare-gen
prepare-dep:
	$(GO) get golang.org/x/tools/cmd/goyacc
prepare-gen:
	cd "bfe_basic/condition/parser" && $(GOGEN)

# make compile, go build
compile: build
build:
	$(GOBUILD) -ldflags "-X main.version=$(BFE_VERSION)" 

# make test, test your code
test: test-case vet-case
test-case:
	$(GOTEST) -race -cover ./...
vet-case:
	${GOVET} ./...

# make package
package:
	mkdir -p $(OUTDIR)/bin
	mv bfe  $(OUTDIR)/bin
	cp -r conf $(OUTDIR)

# make clean
clean:
	rm -rf $(OUTDIR)
	rm -rf $(WORKROOT)/bfe
	rm -rf $(GOPATH)/pkg/linux_amd64

# avoid filename conflict and speed up build 
.PHONY: all prepare compile test package clean build
