# SPDY

## Introduction

spdy_state monitor state of SPDY.

## Monitor Item

| Monitor Item                   | Description                                             |
| ------------------------------ | ------------------------------------------------------- |
| SPDY_CONN_OVERLOAD             | Counter for exceeding connection rate limit             |
| SPDY_ERR_BAD_REQUEST           | Counter for bad request                                 |
| SPDY_ERR_FLOW_CONTROL          | Counter for flow control                                |
| SPDY_ERR_GOT_RESET             | Counter for gettting RST_STREAM                         |
| SPDY_ERR_INVALID_DATA_STREAM   | Counter for invalid data stream                         |
| SPDY_ERR_INVALID_SYN_STREAM    | Counter for invalid SYN stream                          |
| SPDY_ERR_MAX_STREAM_PER_CONN   | Counter for reaching advertised concurrent stream limit |
| SPDY_ERR_NEW_FRAMER            | Counter for creating frame failed                       |
| SPDY_ERR_STREAM_ALREADY_CLOSED | Counter for stream already closed                       |
| SPDY_ERR_STREAM_CANCEL         | Counter for canceling stream                            |
| SPDY_PANIC_CONN                | Counter for connection panic                            |
| SPDY_PANIC_STREAM              | Counter for stream panic                                |
| SPDY_REQ_HEADER_COMPRESS_SIZE  | Size of request header before compress                  |
| SPDY_REQ_HEADER_ORIGINAL_SIZE  | Size of request header after compress                   |
| SPDY_REQ_OVERLOAD              | Counter for exceeding request rate limit                |
| SPDY_RES_HEADER_COMPRESS_SIZE  | Size of response header before compress                 |
| SPDY_RES_HEADER_ORIGINAL_SIZE  | Size of response header after compress                  |
| SPDY_TIMEOUT_CONN              | Timeout of SPDY connection                              |
| SPDY_TIMEOUT_READ_STREAM       | Timeout waiting for reading stream                      |
| SPDY_TIMEOUT_WRITE_STREAM      | Timeout waiting for writing stream                      |
| SPDY_UNKNOWN_FRAME             | Counter for unknown frame                               |
