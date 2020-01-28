# 编译和运行

## 编译
### 环境准备
- golang 1.13+
- git

### 源代码下载

- BFE代码位于如下repo中:

    [https://github.com/baidu/bfe](https://github.com/baidu/bfe)

- clone代码
    ```
    $ mkdir -p github.com/baidu
    $ cd github.com/baidu
    $ git clone https://github.com/baidu/bfe
    $ cd bfe
    ```

### 源代码编译

- 在BFE源代码目录(github.com/baidu/bfe)运行命令：
    ```
    $ make
    ```

- 运行如下命令可执行测试:
    ```
    $ make test
    ```

- 可执行目标生成于
    ```
    $ file output/bin/bfe

    output/bin/bfe: ELF 64-bit LSB executable, ...
    ```

## 运行BFE

### 配置文件

- 示例配置文件位于目录conf中

### 运行BFE
- 运行生成的BFE可执行文件
    ```
    $ cd output/bin/
    $ ./bfe -c ../conf -l ../log
    ```

