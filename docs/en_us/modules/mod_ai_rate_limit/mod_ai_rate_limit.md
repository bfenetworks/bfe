# mod_ai_rate_limit

## Introduction

mod_ai_rate_limit performs rate limiting on AI requests. It supports distributed rate limiting based on Redis, and can configure TPM (tokens per minute), RPM (requests per minute), and max concurrency limits by dimensions such as product and apikey.

## Module Configuration

- [mod_ai_rate_limit.conf](../../configuration/mod_ai_rate_limit/mod_ai_rate_limit.conf.md)

## Rule Configuration

- [ai_rate_limit.data](../../configuration/mod_ai_rate_limit/ai_rate_limit.data.md)
