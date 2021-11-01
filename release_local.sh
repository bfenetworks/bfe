#/bin/bash

go test ./...
if [ $? -ne 0 ]; then
	exit 1;
fi

goreleaser release --skip-publish --skip-validate --rm-dist
