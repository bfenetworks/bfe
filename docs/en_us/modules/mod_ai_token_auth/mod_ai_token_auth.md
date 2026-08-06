# mod_ai_token_auth

## Module Overview

mod_ai_token_auth supports API-key (token) authentication for LLM services. An API-key represents a token with certain access permissions and quotas for specific LLM services. This module checks the API-key carried in the request according to rules to determine whether the request is allowed to access the LLM service.

Request header carries the API-key:
```
Authorization: Bearer <api-key>
```

## Configuration

- [Basic Configuration](../../configuration/mod_ai_token_auth/mod_ai_token_auth.conf.md)
- [Rule Configuration](../../configuration/mod_ai_token_auth/token_rule.data.md)

## Working Principle

### Token Authentication Flow

1. **Request Entry**: When a request arrives, the module checks if it matches the authentication rule
2. **Token Validation**: Extracts the API-key from the `Authorization` header of the request and validates its validity (status, expiration time, etc.)
3. **Model Permission Check**: Verifies whether the requested model is in the allowed list and not in the blocked list
4. **IP Subnet Check**: Verifies whether the request source IP is within the allowed subnet range
5. **Quota Check**: Checks if the associated quota plans have sufficient quota
6. **Quota Deduction**: After the request completes, extracts the token usage from the response body and deducts the corresponding quota

### Monitoring Metrics

| Metric Name | Type | Description |
| ----------- | ---- | ----------- |
| REQ_TOTAL | Counter | Total number of requests |
| REQ_AUTH | Counter | Number of requests triggering authentication |
| REQ_AUTH_FAIL | Counter | Number of authentication failures |
