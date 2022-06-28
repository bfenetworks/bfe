# 在OpenBSD环境下安装BFE

下文以OpenBSD 6.6及BFE 0.4.0版本为例，说明安装流程

## 环境说明

* 设置OpenBSD 6.6软件源安装路径并安装相关软件包：

amd64

```bash
# export PKG_PATH="https://mirrors.tuna.tsinghua.edu.cn/OpenBSD/6.6/packages/amd64/"
# pkg_add wget go
```

i386

```bash
# export PKG_PATH="https://mirrors.tuna.tsinghua.edu.cn/OpenBSD/6.6/packages/i386/"
# pkg_add llvm wget go
```

* 由于OpenBSD 6.6自带的make 无法编译BFE，因此需要安装gnu make

amd64

```bash
# wget http://ftp.gnu.org/gnu/make/make-4.2.tar.bz2
# tar -xvjf make-4.2.tar.bz2
# cd make-4.2
# ./configure
# make -j8
# make install
```

i386

```bash
# cd /usr/bin
# ln -s clang gcc
#
# wget http://ftp.gnu.org/gnu/make/make-4.2.tar.bz2
# tar -xvjf make-4.2.tar.bz2
# cd make-4.2
# ./configure
# make -j8 
# make install
```

## 编译安装BFE

* 下载bfe 0.4.0 并编译安装

```bash
# wget https://github.com/bfenetworks/bfe/archive/v0.4.0.tar.gz
# tar -xvzf v0.4.0.tar.gz
# cd bfe-0.4.0/
# export GOPROXY=https://goproxy.io
# /usr/local/bin/make -j8   
#
# mkdir -p /usr/local/bfe/bin
# cp bfe /usr/local/bfe/bin
# cp -fr conf/ /usr/local/bfe
```

* 修改配置文件

```bash
# cd /usr/local/bfe/conf/mod_access/
# vi mod_access.conf
LogDir =  ../log
:wq ## 保存退出
```

* 创建启动脚本及运行

```bash
# mkdir /root/run_bfe
# cd /root/run_bfe
# vi run_bfe.sh
#!/usr/local/bin/bash
cd /usr/local/bfe/bin
./bfe -c ../conf -l ../log &
:wq ## 保存退出

# chmod 755 run_bfe.sh
# sh run_bfe.sh
# ps |grep bfe
25625 p0  I        6:08.02 ./bfe -c ../conf -l ../log
79047 p0  R+/2     0:00.00 grep bfe
```
