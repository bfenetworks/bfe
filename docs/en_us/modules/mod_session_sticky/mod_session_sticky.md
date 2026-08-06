# mod_session_sticky

## Introduction

mod_session_sticky implements session sticky functionality, ensuring that requests from the same user are always routed to the same backend server during the session lifetime.

This module supports two session sticky modes:

- **Cookie mode**: Backend information is encrypted and stored in a Cookie. The client carries this Cookie with each request, and BFE routes the request to the corresponding backend based on the Cookie content.
- **Sticky mode**: Maintains a mapping between stickyid and backend through a cache mechanism. Supports getting stickyid from Cookie, request header, or URL parameter.

## Module Configuration

- conf/mod_session_sticky/mod_session_sticky.conf: See [mod_session_sticky Basic Configuration](../../configuration/mod_session_sticky/mod_session_sticky.conf.md).

## Rule Configuration

- conf/mod_session_sticky/session_sticky.data: See [mod_session_sticky Rule Configuration](../../configuration/mod_session_sticky/session_sticky.data.md).

## Working Principle

### Cookie Mode

1. **Encode**: When a request first arrives without session sticky information, BFE selects a backend and encrypts the backend information (address, port, subcluster) into a Cookie, which is returned to the client.
2. **Decode**: Subsequent requests from the client carry this Cookie. BFE decrypts the Cookie content, retrieves the backend information, and routes the request to the corresponding backend.

### Sticky Mode

1. **Encode**: When a request first arrives, BFE selects a backend and stores the mapping between stickyid (obtained from Cookie or JSON response body) and backend information in the cache.
2. **Decode**: Subsequent requests from the client carry the same stickyid (obtained from Cookie, request header, URL parameter, or JSON request body). BFE looks up the corresponding backend information from the cache and routes the request to the corresponding backend.

### Cache Types

The module supports two cache types:

- **local**: Uses LRU cache to store the mapping between stickyid and backend. Suitable for single-node deployment or scenarios where session sharing across nodes is not required.
- **redis**: Uses Redis to store the mapping. Suitable for multi-node deployment scenarios to ensure session sharing across different nodes.

### JSON Request/Response Fields

For OpenAI-compatible interfaces and similar scenarios, the module supports extracting stickyid from JSON request and response bodies:

- **StickyRequestField**: Extracts stickyid from JSON request body, for example, from the `previous_response_id` field.
- **StickyResponseField**: Extracts stickyid from JSON response body, for example, from the `response_id` field.

Extraction priority (Decode phase): Cookie > Header > URIParam > StickyRequestField

Extraction priority (Encode phase): Cookie > StickyResponseField

### Cookie Renewal Mechanism

When the remaining validity period of the Cookie is less than `RenewWindow`, BFE will reset the Cookie to extend its validity period. This ensures that users do not lose session sticky status due to Cookie expiration during long sessions.

### Mask Code Compatibility

The module supports primary and standby mask codes. If decryption with the primary mask code fails, it will try the standby mask code. This is useful when you need to change the mask code without interrupting existing sessions.

## Metrics

| Metric | Description |
| ------ | ----------- |
| VERSION | Version of currently effective rules |
