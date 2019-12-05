# 在OpenBSD6.6环境下安装百度开源BFE 0.4.0

## 经过验证，可以在OpenBSD 6.6 amd64和i386平台上面安装并成功运行bfe 0.4.0

* 安装OpenBSD 6.6操作系统，这部分略。

* 设置OpenBSD 6.6软件源安装路径并安装相关软件包：

amd64
```
# export PKG_PATH="https://mirrors.tuna.tsinghua.edu.cn/OpenBSD/6.6/packages/amd64/"
# pkg_add wget go
```

i386
```
# export PKG_PATH="https://mirrors.tuna.tsinghua.edu.cn/OpenBSD/6.6/packages/i386/"
# pkg_add llvm wget go

```

* 由于OpenBSD 6.6自带的make 无法编译BFE 0.4.0，因此需要手动编译安装gnu make

amd64
```
# wget http://ftp.gnu.org/gnu/make/make-4.2.tar.bz2
# tar -xvjf make-4.2.tar.bz2
# cd make-4.2
# ./configure
# make -j8
# make install
```
i386
```
# cd /usr/bin
# ln -s clang gcc
# cd /root
# wget http://ftp.gnu.org/gnu/make/make-4.2.tar.bz2
# tar -xvjf make-4.2.tar.bz2
# cd make-4.2
# ./configure
# make -j8 
# make install
```

* 下载bfe 0.4.0 并编译安装
```
# cd /root
# wget https://github.com/baidu/bfe/archive/v0.4.0.tar.gz
# tar -xvzf v0.4.0.tar.gz
# cd bfe-0.4.0/
# export GOPROXY=https://goproxy.io
# /usr/local/bin/make -j8   
# 编译过程有告警信息，不用理会。
# mkdir -p /usr/local/baidu_bfe/bin
# cp bfe /usr/local/baidu_bfe/bin
# cp -fr conf/ /usr/local/baidu_bfe
```

* 修改配置文件和创建启动脚本
```
# cd /usr/local/baidu_bfe/conf/mod_access/
# vi mod_access.conf
LogDir =  ../log
:wq #### 保存退出
# mkdir /root/run_bfe
# cd /root/run_bfe
# vi run_bfe.sh
#!/usr/local/bin/bash
cd /usr/local/baidu_bfe/bin
./bfe -c ../conf -l ../log &
:wq #### 保存推出
# chmod 755 run_bfe.sh
# sh run_bfe.sh
# ps |grep bfe
25625 p0  I        6:08.02 ./bfe -c ../conf -l ../log
79047 p0  R+/2     0:00.00 grep bfe
#### bfe 0.4.0 已经在OpenBSD 6.6上成功运行了。
```
