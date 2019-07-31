# 协议相关

- **req_proto_secure()**
  - 判断请求是否基于安全传输协议，包括https/spdy/http2 
  - 如果请求基于http, 返回false；如果请求基于https/spdy/http2，返回true