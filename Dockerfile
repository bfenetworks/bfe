# Copyright 2021 The BFE Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
FROM golang:1.17.5-alpine3.15 AS build

WORKDIR /bfe
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=`cat VERSION`"

FROM alpine:3.15 AS run
RUN apk update && apk add --no-cache ca-certificates
COPY --from=build /bfe/bfe /bfe/bin/
COPY conf /bfe/conf/
EXPOSE 8080 8443 8421

WORKDIR /bfe/bin
ENTRYPOINT ["./bfe"]
CMD ["-c", "../conf/", "-l", "../log"]
