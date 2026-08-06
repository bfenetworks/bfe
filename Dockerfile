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
FROM --platform=${BUILDPLATFORM} golang:1.22.2-alpine3.19 AS build
ARG TARGETARCH
ARG TARGETOS

WORKDIR /src
COPY . .

RUN set -ex; \
	mkdir -p /out; \
	CGO_ENABLED=0 \
	GOOS=${TARGETOS:-linux} \
	GOARCH=${TARGETARCH:-$(go env GOARCH)} \
	go build -ldflags "-X main.version=$(cat VERSION)" -o /out/bfe

FROM alpine:3.19 AS confagent
ARG TARGETARCH
ARG CONF_AGENT_VERSION=0.0.2
ARG CONF_AGENT_REPO=bfenetworks

RUN apk add --no-cache ca-certificates wget tar

RUN set -ex; \
	CONF_AGENT_VERSION_NO_V="${CONF_AGENT_VERSION#v}"; \
	CONF_AGENT_VERSION_TAG="v${CONF_AGENT_VERSION_NO_V}"; \
	ARCH="${TARGETARCH:-}"; \
	if [ -z "${ARCH}" ]; then \
		ARCH="$(uname -m)"; \
	fi; \
	case "${ARCH}" in \
		amd64|x86_64) CONF_AGENT_ARCH="amd64" ;; \
		arm64|aarch64) CONF_AGENT_ARCH="arm64" ;; \
		*) echo "Unsupported architecture: ${ARCH}"; exit 1 ;; \
	esac; \
	CONF_AGENT_URL="https://github.com/${CONF_AGENT_REPO}/conf-agent/releases/download/${CONF_AGENT_VERSION_TAG}/conf-agent_${CONF_AGENT_VERSION_NO_V}_linux_${CONF_AGENT_ARCH}.tar.gz"; \
	wget -O /tmp/conf-agent.tar.gz "${CONF_AGENT_URL}"; \
	tar -xzf /tmp/conf-agent.tar.gz -C /tmp; \
	mkdir -p /out; \
	mv /tmp/conf-agent /out/conf-agent; \
	if [ -d /tmp/conf ]; then mv /tmp/conf /out/conf-agent-conf; else mkdir -p /out/conf-agent-conf; fi; \
	chmod +x /out/conf-agent

FROM alpine:3.19 AS logreader
ARG TARGETARCH
ARG LOG_READER_VERSION=1.0.0
ARG LOG_READER_REPO=bfenetworks

RUN apk add --no-cache ca-certificates wget tar

RUN set -ex; \
	LOG_READER_VERSION_NO_V="${LOG_READER_VERSION#v}"; \
	LOG_READER_VERSION_TAG="v${LOG_READER_VERSION_NO_V}"; \
	ARCH="${TARGETARCH:-}"; \
	if [ -z "${ARCH}" ]; then \
		ARCH="$(uname -m)"; \
	fi; \
	case "${ARCH}" in \
		amd64|x86_64) LR_ARCH="amd64" ;; \
		arm64|aarch64) LR_ARCH="arm64" ;; \
		*) echo "Unsupported architecture: ${ARCH}"; exit 1 ;; \
	esac; \
	URL="https://github.com/${LOG_READER_REPO}/log-reader/releases/download/${LOG_READER_VERSION_TAG}/log-reader_${LOG_READER_VERSION_NO_V}_linux_${LR_ARCH}.tar.gz"; \
	wget -O /tmp/log-reader.tar.gz "${URL}"; \
	tar -xzf /tmp/log-reader.tar.gz -C /tmp; \
	mkdir -p /out/bin /out/conf; \
	# try expected top-level dir first, fallback to flat extraction \
	LOG_READER_SRC="/tmp/log-reader_${LOG_READER_VERSION_NO_V}_linux_${LR_ARCH}"; \
	if [ ! -f "${LOG_READER_SRC}/bin/log-reader" ]; then \
		LOG_READER_SRC="/tmp"; \
	fi; \
	cp "${LOG_READER_SRC}/bin/log-reader" /out/bin/; \
	cp -r "${LOG_READER_SRC}/conf/"* /out/conf/; \
	chmod +x /out/bin/log-reader

FROM alpine:3.19
ARG VARIANT=prod

RUN set -ex; \
	apk add --no-cache ca-certificates tini; \
	if [ "${VARIANT}" = "debug" ]; then \
		apk add --no-cache bash curl wget vim; \
	fi

RUN mkdir -p /home/work/conf-agent/conf \
	&& mkdir -p /home/work/conf-agent/log \
	&& mkdir -p /home/work/bfe/bin \
	&& mkdir -p /home/work/bfe/conf \
	&& mkdir -p /home/work/bfe/log \
	&& mkdir -p /home/work/log-reader/bin \
	&& mkdir -p /home/work/log-reader/conf/mod_kafka \
	&& mkdir -p /home/work/log-reader/log

COPY --from=confagent /out/conf-agent /home/work/conf-agent/conf-agent
COPY --from=confagent /out/conf-agent-conf /home/work/conf-agent/conf
COPY --from=build /out/bfe /home/work/bfe/bin/bfe
COPY --from=build /src/conf /home/work/bfe/conf/
COPY --from=logreader /out/bin/log-reader /home/work/log-reader/bin/log-reader
COPY --from=logreader /out/conf/ /home/work/log-reader/conf/

RUN set -ex; \
	if [ -f /home/work/bfe/conf/server_data_conf/name_conf.data ]; then \
		mv /home/work/bfe/conf/server_data_conf/name_conf.data /home/work/bfe/conf/name_conf.data; \
		ln -s /home/work/bfe/conf/name_conf.data /home/work/bfe/conf/server_data_conf/name_conf.data; \
	fi

# Override log-reader v1.0.0 release defaults for bfe image integration:
#   - LogFile: relative path -> absolute path
#   - Brokers: hardcoded IP -> service name (compose container_name + K8s namespace-qualified DNS)
RUN set -ex; \
	sed -i 's|^LogFile = .*|LogFile = /home/work/bfe/log/pb_access3.log|' \
		/home/work/log-reader/conf/config.conf; \
	sed -i 's|^Brokers = .*|Brokers = kafka.ai-gateway-system:9092|' \
		/home/work/log-reader/conf/mod_kafka/mod_kafka.conf

# COPY deploy/docker/entrypoint.sh /home/work/entrypoint.sh
# Generate entrypoint.sh inside the image to avoid external file dependency
RUN set -ex; \
    cat > /home/work/entrypoint.sh <<'EOF'
#!/bin/sh
set -eu

CONF_AGENT_PID=""
LOG_READER_PID=""

# Log function
log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

# Start conf-agent in background
start_conf_agent() {
    if [ -f "/home/work/conf-agent/conf-agent" ]; then
        log "Starting conf-agent..."
        cd /home/work/conf-agent
        nohup ./conf-agent -c ./conf </dev/null >./log/stdout.log 2>&1 &
        CONF_AGENT_PID=$!
        log "conf-agent started, PID: $CONF_AGENT_PID"
        cd /home/work
    else
        log "Warning: conf-agent binary not found, skipping startup"
    fi
}

# Start log-reader in background
start_log_reader() {
    if [ -f "/home/work/log-reader/bin/log-reader" ]; then
        log "Starting log-reader..."
        cd /home/work/log-reader
        nohup ./bin/log-reader -c ./conf -l ./log </dev/null >./log/stdout.log 2>&1 &
        LOG_READER_PID=$!
        log "log-reader started, PID: $LOG_READER_PID"
        cd /home/work
    else
        log "Warning: log-reader binary not found, skipping startup"
    fi
}

# Start bfe in background
start_bfe() {
    log "Starting bfe..."
    cd /home/work/bfe/bin
    nohup ./bfe -c ../conf/ -l ../log/ -s__BFE_DEBUG_FLAG__ </dev/null >../log/stdout.log 2>&1 &
    BFE_PID=$!
    log "bfe started, PID: $BFE_PID"
}

# Signal handler
cleanup() {
    log "Received termination signal, shutting down..."

    if [ -n "$BFE_PID" ]; then
        log "Stopping bfe (PID: $BFE_PID)..."
        kill -TERM "$BFE_PID" 2>/dev/null || true
    fi
    if [ -n "$LOG_READER_PID" ]; then
        log "Stopping log-reader (PID: $LOG_READER_PID)..."
        kill -TERM "$LOG_READER_PID" 2>/dev/null || true
    fi
    if [ -n "$CONF_AGENT_PID" ]; then
        log "Stopping conf-agent (PID: $CONF_AGENT_PID)..."
        kill -TERM "$CONF_AGENT_PID" 2>/dev/null || true
    fi

    wait 2>/dev/null || true
    log "Shutdown complete"
    exit 0
}

# Register signal handlers
trap cleanup TERM INT

# Main process
log "========================================"
log "BFE Container Startup (bfe + conf-agent + log-reader)"
log "========================================"

# 1. Start conf-agent
start_conf_agent

# 2. Start log-reader
start_log_reader

# Wait for conf-agent initialization
sleep 2

# 3. Start bfe
start_bfe

# Block until a child exits or signal received
wait
EOF

RUN set -ex; \
	if [ "${VARIANT}" = "debug" ]; then \
		sed -i 's/__BFE_DEBUG_FLAG__/ -d debug/g' /home/work/entrypoint.sh; \
	else \
		sed -i 's/__BFE_DEBUG_FLAG__//g' /home/work/entrypoint.sh; \
	fi

RUN chmod +x /home/work/entrypoint.sh

EXPOSE 8080 8443 8421 8992

WORKDIR /home/work
ENTRYPOINT ["/sbin/tini", "--", "/home/work/entrypoint.sh"]
