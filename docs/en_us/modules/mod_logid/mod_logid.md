# mod_logid

## Introduction

mod_logid generates log ids for sessions/requests.

## Module Configuration

Not required

## Metrics

| Metric                  | Description                                                      |
| ----------------------- | ---------------------------------------------------------------- |
| NO_LOGID_FROM_UPPER_BFE | Counter of trusted-source requests that arrived without a log id |

## Log id format

Since bfenetworks/bfe#1417 a log id is a **decimal string** denoting a value in
`[1, 2^63-1]`; previously it was a 32-char hex string (128 bits), which could not
be carried by the `uint64` `logid` field of the access log and was silently
written as 0. The access log stores the same value, so the string and the field
convert into each other losslessly (`logid_str == FormatUint(logid, 10)`). The
63-bit bound keeps the value inside the signed `BIGINT` of the report databases
(MySQL / Doris / StarRocks).
