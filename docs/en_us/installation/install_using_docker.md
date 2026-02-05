# Install using Docker

This document explains how to run BFE with Docker, and how to build/push Docker images from source.

## Option 1: Run a prebuilt image

If you already have an image (for example `ghcr.io/bfenetworks/bfe`, or an image you built and pushed to a private registry), you can run it directly:

```bash
docker run --rm \
	-p 8080:8080 -p 8443:8443 -p 8421:8421 \
	<your-image>
```

Example:

```bash
docker run --rm \
	-p 8080:8080 -p 8443:8443 -p 8421:8421 \
	ghcr.io/bfenetworks/bfe:latest
```

You can access:
- http://127.0.0.1:8080/ (may return 500 if no rule matches)
- http://127.0.0.1:8421/monitor (monitoring endpoint)

## Option 2: Build images from source (recommended)

From the repository root:

```bash
# Build both prod and debug images
make docker

# Optional: override image name (default: bfe)
make docker BFE_IMAGE_NAME=bfe
```

Notes:
- Image tags are derived from the `VERSION` file and normalized to start with `v` (for example `1.8.0` becomes `v1.8.0`).
- `make docker` is a local build and does not require buildx.

Example tags when `VERSION=1.8.0`:
- `bfe:v1.8.0` (prod)
- `bfe:v1.8.0-debug` (debug)
- `bfe:latest` (always points to prod)

## Option 3: Build and push images to a registry (make docker-push)

If you want Kubernetes (or other machines) to pull your image, use `make docker-push` to build and push multi-arch images (default platforms: `linux/amd64,linux/arm64`).

Prerequisites:
- A registry you can push to (GHCR, Harbor, Docker Hub, etc.)
- You have logged in via `docker login <registry>`
- Docker buildx is available (Docker Desktop usually includes it)

Common variables:
- `REGISTRY`: required, registry prefix (for example `ghcr.io/your-org`)
- `BFE_IMAGE_NAME`: image name (default: `bfe`, can also be `team/bfe`)
- `PLATFORMS`: build platforms (default: `linux/amd64,linux/arm64`)

Example: push to GHCR (result: `ghcr.io/cc14514/bfe:<tag>`):

```bash
make docker-push REGISTRY=ghcr.io/cc14514
```

Example: push to a private registry and build only amd64:

```bash
make docker-push \
	REGISTRY=registry.example.com \
	BFE_IMAGE_NAME=infra/bfe \
	PLATFORMS=linux/amd64
```

After pushing (example `VERSION=1.8.0`):
- `$(REGISTRY)/$(BFE_IMAGE_NAME):v1.8.0` (prod, multi-arch)
- `$(REGISTRY)/$(BFE_IMAGE_NAME):v1.8.0-debug` (debug, multi-arch)
- `$(REGISTRY)/$(BFE_IMAGE_NAME):latest` (prod, multi-arch)

If you deploy via the Kubernetes example and want to use your pushed image, update the bfe image mapping under `images:` in `examples/kubernetes/kustomization.yaml`.

## Customize configuration (mount local directories)

Paths inside the image:
- BFE config: `/home/work/bfe/conf`
- BFE logs: `/home/work/bfe/log`
- conf-agent config: `/home/work/conf-agent/conf`
- conf-agent logs: `/home/work/conf-agent/log`

Example (adjust paths as needed):

```bash
docker run --rm \
	-p 8080:8080 -p 8443:8443 -p 8421:8421 \
	-v /Users/BFE/Desktop/conf:/home/work/bfe/conf \
	-v /Users/BFE/Desktop/log:/home/work/bfe/log \
	-v /Users/BFE/Desktop/conf-agent:/home/work/conf-agent/conf \
	bfe:latest
```

## Further reading

- Get familiar with [Command options](../operation/command.md)
- Get started with [Beginner's Guide](../example/guide.md)
