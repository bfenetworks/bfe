# Configuration overview

## Notes

This document explain how to configure BFE Server directly.

For guide of configure BFE via BFE control plane components, see documents of [BFE API Server](https://github.com/bfenetworks/api-server) and [BFE Dashboard](https://github.com/bfenetworks/dashboard).

## Configuration types

- Normal configuration: For changes to the configuration file to take effect, you must restart the bfe process.
- Dynamic configuration: For changes to the configuration file to take effect, you can either restart or reload the bfe process.

## Configuration format

- Normal configuration file: INI format
- Dynamic configuration file: JSON format (except for cerfificate/dict file, etc)

## Configuration layout

The main configuration file is named bfe.conf (conf/bfe.conf). To make the configuration easier to maintain,
we split it into a set of feature-specific files stored in the conf/&#60;feature&#62;/ directory.

| Category     | Layout   |
| ------------ | -------- |
| Main configuration | conf/bfe.conf |
| Configuration about protocol | conf/tls_conf/ |
| Configuration about routing | conf/server_data_conf/ |
| Configuration about balancing | conf/cluster_conf/ |
| Configuration about modules | conf/mod_&#60;name&#62; |

## Reloading Configuration

See [Reloading configuration](../operation/reload.md)
