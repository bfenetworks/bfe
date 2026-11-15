# mod_access_pb3 Data Configuration

## Introduction

`mod_access_pb.data` is the dynamic configuration file of the `mod_access_pb3` module. It configures optional collection policies of the access log (currently: collecting request/response headers by whitelist into the proto fields `req_headers`/`res_headers`).

The file path is specified by `BasicConf.DataConfPath` in `mod_access_pb3.conf`. It **supports hot reload**: after modifying the file, call the BFE monitor reload API (`/reload/mod_access_pb3`) to take effect without restarting. When the file is absent or a section is missing, the module falls back to default behavior (no whitelisted fields are collected), identical to a deployment without this capability.

## Configuration Description

| Configuration Item | Type | Meaning | Required | Supplementary Description | Validity Condition |
| ------------------ | ---- | ------- | -------- | ------------------------- | ------------------ |
| Version | String | Configuration file version | Y | Usually a timestamp, e.g. `20261115090000` | Type is [Version](../00-common.md#5-version) |
| access_header | Object | Request/response header whitelist configuration | N | Absent means no headers are collected | - |
| access_header.req_headers | Array | Names of request headers to collect | N | Name matching is **case-insensitive** (compared after HTTP canonical form normalization); collected values are written to the proto field `req_headers` (`repeated HttpHeader{key,value}`) | Each element must be a valid HTTP header name (RFC 7230 token characters), length after trimming ≤ 128 bytes |
| access_header.res_headers | Array | Names of response headers to collect | N | Same as above, written to the proto field `res_headers` | Same as above |

Notes:

- Header name matching is case-insensitive: `business-type` and `Business-Type` are equivalent; the `key` written to proto is always in HTTP canonical form (e.g. `Business-Type`);
- Multiple values of the same header are joined with `,` into a single value (consistent with existing standalone fields); values longer than 2048 bytes are truncated;
- Unknown top-level keys (sections) in the file are ignored with a Warn log and do not affect loading of known sections (forward compatibility);
- The whitelist content is not restricted for sensitive headers; the raw API Key is masked at runtime by the module (`maskSensitiveCredentials`) and never persists in any field.

## Configuration Example

```json
{
    "Version": "20261115090000",
    "access_header": {
        "req_headers": ["business_type", "x-request-id"],
        "res_headers": ["x-trace-id"]
    }
}
```
