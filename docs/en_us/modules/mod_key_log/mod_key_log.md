# mod_key_log

## Introduction

mod_key_log writes tls key logs in NSS key log format so that external
programs(eg. wireshark) can decrypt TLS connections for trouble shooting.

For more information about NSS key log format, see:
https://developer.mozilla.org/en-US/docs/Mozilla/Projects/NSS/Key_Log_Format

## Module Configuration

- [mod_key_log.conf](../../configuration/mod_key_log/mod_key_log.conf.md)
