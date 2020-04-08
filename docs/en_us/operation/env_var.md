# Environment arguments

## GODEBUG

* export GODEBUG="http2debug=1"
    * output verbose log which prints header info processed by http2 hpack encode

* export GODEBUG="http2debug=2"
    * output verbose log which prints header info processed by http2 hpack encode
    * output verbose framer log read or wrote by http2 server 
