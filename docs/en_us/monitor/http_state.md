# HTTP

## Introduction

http_state monitor state of HTTP1.0/1.1.

## Monitor Item

| Monitor Item                 | Description                                                  |
| ---------------------------- | ------------------------------------------------------------ |
| HTTP_BACKEND_CONN_ALL        | Counter for connecting with backend                          |
| HTTP_BACKEND_CONN_SUCC       | Counter for connecting successfully with backend             |
| HTTP_BACKEND_REQ_ALL         | Counter for sending request to backend                       |
| HTTP_BACKEND_REQ_SUCC        | Counter for sending successfully request to backend          |
| HTTP_CANCEL_ON_CLIENT_CLOSE  | Counter for closing connection when finding client disconnected |
| HTTP_PANIC_BACKEND_READ      | Counter for reading backend panic                            |
| HTTP_PANIC_BACKEND_WRITE     | Counter for writing backend panic                            |
| HTTP_PANIC_CLIENT_FLUSH_LOOP | Counter for client flushing loop panic                       |
| HTTP_PANIC_CLIENT_WATCH_LOOP | Counter for client watching loop panic                       |

