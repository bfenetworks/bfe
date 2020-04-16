# 环境变量说明

## GODEBUG

* export GODEBUG="http2debug=1"
    * 输出经http2 hpack encode处理的原始header日志信息（http/1.1）

* export GODEBUG="http2debug=2"
    * 输出经http2 hpack encode处理的原始header日志信息（http/1.1）
    * 输出读取和发送的http2 frame日志信息
