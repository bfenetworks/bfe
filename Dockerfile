FROM golang:1.13.4-alpine AS build

WORKDIR /bfe
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=`cat VERSION`"

FROM alpine:3.10 AS run
RUN apk update && apk add --no-cache ca-certificates
COPY --from=build /bfe/bfe /bfe/output/bin/
COPY conf /bfe/output/conf/
EXPOSE 8080 8443 8421

WORKDIR /bfe/output/bin
CMD ["./bfe", "-c", "../conf/"]
