# Copyright (c) 2020 The BFE Authors.
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

WORKROOT := $(shell pwd)
OUTDIR   := $(WORKROOT)/output

all: build

# build content for website
build:
	mkdocs build -f mkdocs_en.yml -d $(OUTDIR)/en_us
	cp -R images $(OUTDIR)/en_us 
	mkdocs build -f mkdocs_zh.yml -d $(OUTDIR)/zh_cn
	cp -R images $(OUTDIR)/zh_cn

clean:
	rm -rf $(OUTDIR)
