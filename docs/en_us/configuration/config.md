# Configuration overview

## Configuration types
- Normal configuration: configuration that requires restart to take effort after modification.
- Dynamic configuration: configuration that requires reload to take effort after modification.

## Configuration format
- Normal configuration file: INI format
- Dynamic configuration file: JSON format (except for cerfificate/dict file, etc)

## Configuration layout
| functional category | configuration layout |
| ------------ | -------- |
| main configuration | conf/bfe.conf |
| configuration about protocol | conf/tls_conf/ | 
| configuration about routing | conf/server_data_conf/ |
| configuration about balancing | conf/cluster_conf/ |
| configuration about modules | conf/mod_&#60;name&#62; |
