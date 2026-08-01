# mod_ai_route

## Introduction

mod_ai_route routes AI requests to different backend clusters and models based on AI routing rules. It supports three types of routing tables: apikey, entity, and global.

## Module Configuration

- [mod_ai_route.conf](../../configuration/mod_ai_route/mod_ai_route.conf.md)

## Rule Configuration

- [ai_route.data](../../configuration/mod_ai_route/ai_route.data.md)

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_TOTAL | Total count of requests |
| REQ_HIT_APIKEY | Count of requests hitting apikey routing |
| REQ_HIT_ENTITY | Count of requests hitting entity routing |
| REQ_HIT_GLOBAL | Count of requests hitting global routing |
| REQ_MISS | Count of requests missing all routing |
| REQ_FALLBACK | Count of requests hitting fallback |
