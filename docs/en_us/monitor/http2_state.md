# HTTP2

## Introduction

The endpoint `/monitor/http2_state` exposes metrics about HTTP2 protocol.

## Metrics

| Metric                      | Description                                             |
| --------------------------- | ------------------------------------------------------- |
| H2_ERR_MAX_HEADER_LIST_SIZE | Counter for reaching max size of header list            |
| H2_ERR_MAX_HEADER_URI_SIZE  | Counter for reaching max size of header URI             |
| H2_ERR_MAX_STREAM_PER_CONN  | Counter for reaching advertised concurrent stream limit |
| H2_ERR_GOT_RESET            | Counter for getting RST_STREAM                         |
| H2_PANIC_CONN               | Counter for connection panic                            |
| H2_PANIC_STREAM             | Counter for stream panic                                |
| H2_REQ_HEADER_COMPRESS_SIZE | Size of request header after compress                   |
| H2_REQ_HEADER_ORIGINAL_SIZE | Size of request header before compress                  |
| H2_RES_HEADER_COMPRESS_SIZE | Size of response header after compress                  |
| H2_RES_HEADER_ORIGINAL_SIZE | Size of response header before compress                 |
| H2_TIMEOUT_CONN             | Counter for timeout of HTTP2 connection timeout         |
| H2_TIMEOUT_PREFACE          | Counter for timeout of waiting for client preface       |
| H2_TIMEOUT_READ_STREAM      | Counter for timeout of waiting for reading stream       |
| H2_TIMEOUT_SETTING          | Counter for timeout of waiting for SETTINGS frames      |
| H2_TIMEOUT_WRITE_STREAM     | Counter for timeout of waiting for writing stream       |
