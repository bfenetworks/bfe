# Protocol

- **req_proto_secure()**
  - Judge if request is based on secure protocol, secure protocol includes https/spdy/http2 
  - If protocol is http, return false; if protocol is https/spdy/http2, return true