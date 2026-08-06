# mod_markdown

## Introduction

mod_markdown renders markdown files to HTML in real time and returns the result to the client.

## Module Configuration

- [mod_markdown.conf](../../configuration/mod_markdown/mod_markdown.conf.md)

## Rule Configuration

- [markdown_rule.data](../../configuration/mod_markdown/markdown_rule.data.md)

## Metrics

| Metric | Description |
| ------ | ----------- |
| REQ_TOTAL | Total count of requests |
| REQ_MARK_DOWN_RULE_HIT | Count of requests hitting markdown rule |
| RSP_RENDER_SUCCESS | Count of successful rendering responses |
| RSP_RENDER_FAILURE | Count of failed rendering responses |
| RSP_RENDER_IGNORE | Count of ignored rendering responses |
| ERR_COUNT_READ_FAIL | Count of read failures |
| ERR_COUNT_RENDER_FAIL | Count of render failures |
