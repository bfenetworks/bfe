<!--
This changelog should always be read on `master` branch. Its contents on other branches
does not necessarily reflect the changes.
-->

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.5.0] - 2019-12-12
### Added
- Add mod_geo to determine user geolocation by MaxMind database
- Add mod_auth_basic to restrict access from unknown users
- Add mod_compress to compress response by customize rules
- Add mod_errors to replace unexpected resposne
- mod_static detect content type based on file extension or content
- Expose pprof items in web monitor
- Add default resFlushInterval for SSE(Server-Sent Events) response
- Support X-Forwarded-Host Header
- Support docker image in Makefile
- Documents optimization

### Changed
- Change default monitor port to 8421
- Rename HANDLE_READ_BACKEND to HANDLE_READ_RESPONSE

### Removed
- Remove some legacy fields from mod_access


## [v0.4.0] - 2019-11-19
### Added
- Expose metrics in prometheus text-based format
- Add default value for core configs to reduce configuration cost
- Add mod_static to serve static content by customize rules
- Documents optimization

### Changed
- mod_logid: random logid in hex encoding format

### Removed
- Remove signal SIGSTKFLT


## [v0.3.0] - 2019-10-17

### Added
- Add condition primitive: req_cip_hash_in/req_header_value_hash_in/req_cookie_value_hash_in/req_query_value_hash_in
- Add mod_header variable: bfe_log_id
- Add mod_http_code to maintain basic status about response forwarded

### Security
- Fix http2: limit number of control frames in server send queue (CVE-2019-9512, CVE-2019-9514)


## [v0.2.0] - 2019-09-26

### Added
- Add proxy protocol to be compatible with F5 BigIP/Citrix ADC etc
- Add mod_access to write request/session log in customized format
- Add mod_key_log to wirte tls key log so that external programs(eg. wireshark) can decrypt TLS connections for trouble shooting
- Add security grade 'A+' in tls
- Add condition primitive: req_query_value_contain/req_header_value_contain/req_cookie_value_contain
- Documents optimization

### Changed
- reverseproxy: flush response header immediately if flushInterval<0


## [v0.1.0] - 2019-08-01

### Added
- Multiple protocols supported, including HTTP, HTTPS, SPDY, HTTP2, WebSocket, TLS, etc
- Content based routing, support user-defined routing rule in advanced domain-specific language
- Support multiple load balancing policies
- Flexible plugin framework to extend functionality. Based on the framework, developer can add new features rapidly
- Detailed built-in metrics available for service status monitor

[v0.5.0]: https://github.com/baidu/bfe/compare/v0.4.0...v0.5.0
[v0.4.0]: https://github.com/baidu/bfe/compare/v0.3.0...v0.4.0
[v0.3.0]: https://github.com/baidu/bfe/compare/v0.2.0...v0.3.0
[v0.2.0]: https://github.com/baidu/bfe/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/baidu/bfe/releases/tag/v0.1.0
