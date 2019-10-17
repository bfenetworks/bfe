# Build and Run

## Build

### Prerequisite
- golang 1.12+
- golang yacc
- git

### Download the source code

- BFE code can be found in following repo:

    [https://github.com/baidu/bfe](https://github.com/baidu/bfe)

- Clone the main BFE repo:
    ```
    $ mkdir -p gocode/src/github.com/baidu
    $ cd gocode/src/github.com/baidu
    $ git clone https://github.com/baidu/bfe
    $ cd bfe
    ```

### Build from source

- Run build script in source directory of bfe (src/github.com/baidu/bfe)：
    ```
    $ make
    ```

- Run the tests:
    ```
    $ make test
    ```

- BFE binary is generated as below:
    ```
    $ file output/bin/bfe

    output/bin/bfe: ELF 64-bit LSB executable ...
    ```

## Run

- Run bfe with the example configurations: conf

    ```
    $ cd output/bin/
    $ ./bfe -c ../conf -l ../log
    ```

