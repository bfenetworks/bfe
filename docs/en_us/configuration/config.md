# Configuration overview

## Configuration types
- Normal configuration: For changes to the configuration file to take effect, you must restart the bfe process.
- Dynamic configuration: For changes to the configuration file to take effect, you can either restart or reload the bfe process.

## Configuration format
- Normal configuration file: INI format
- Dynamic configuration file: JSON format (except for cerfificate/dict file, etc)

## Configuration layout
The main configuration file is named bfe.conf (conf/bfe.conf). To make the configuration easier to maintain, 
we split it into a set of feature-specific files stored in the conf/&#60;feature&#62;/ directory.

| functional category | configuration layout |
| ------------ | -------- |
| main configuration | conf/bfe.conf |
| configuration about protocol | conf/tls_conf/ | 
| configuration about routing | conf/server_data_conf/ |
| configuration about balancing | conf/cluster_conf/ |
| configuration about modules | conf/mod_&#60;name&#62; |

## Reloading Configuration

See [Reloading configuration](operation/reload.md)
