# mod_static

## Introduction

mod_static serves static files.

## Module Configuration

- conf/mod_static/mod_static.conf: See [mod_static Basic Configuration](../../configuration/mod_static/mod_static.conf.md).

## Rule Configuration

- conf/mod_static/static_rule.data: See [mod_static Rule Configuration](../../configuration/mod_static/static_rule.data.md).

## MIME Configuration

- conf/mod_static/mime_type.data: See [mod_static MIME Configuration](../../configuration/mod_static/mime_type.data.md).

## Metrics

| Metric | Description |
| ------ | ----------- |
| FILE_BROWSE_COUNT | Counter for BROWSE requests |
| FILE_CURRENT_OPENED | Counter for current opend files |
| FILE_BROWSE_NOT_EXIST | Counter for "file not exists" requests |
| FILE_BROWSE_SIZE | Total served file size |
| FILE_BROWSE_CONTENT_TYPE_ERROR | Count of Content-Type errors |
| FILE_BROWSE_FALLBACK_DEFAULT | Count of fallback to default file |
