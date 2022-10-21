# BFE Callbacks

## Callback Points in forwarding process

The Callback Points in the forwarding process are shown below.
![BFECallbacks](../../../images/bfe-callback.png)

## List of Callback Points

There are 9 callback points in BFE:

- HandleAccept: after TCP connection with client is established.
- HandleHandshake: after SSL/TLS handshake with client is finished.
- HandleBeforeLocation: before the destination product for the request is identified.
- HandleFoundProduct: after the destination product is identified.
- HandleAfterLocation: after the destination cluster is identified.
- HandleForward: after the destination subcluster is identified, and before the request is forwarded.
- HandleReadResponse: after response from backend is received by BFE.
- HandleRequestFinish: after response from backend is forwarded by BFE.
- HandleFinish: after connection with client is closed.

The definition of callback points is in [/bfe_module/bfe_callback.go](https://github.com/bfenetworks/bfe/tree/master/bfe_module/bfe_callback.go)

## Return Values of Callback Function

BFE takes different actions based on the return values of the callback functions.
The return values and the actions are defined as follows:

- BfeHandlerFinish: send response, then close connection.
- BfeHandlerGoOn: go on to next callback function.
- BfeHandlerRedirect: redirect directly.
- BfeHandlerResponse: send response.
- BfeHandlerClose: close connection without sending response.

The definition of return values is in [/bfe_module/bfe_handler_list.go](https://github.com/bfenetworks/bfe/tree/master/bfe_module/bfe_handler_list.go)

## Types of Callback Functions

The format of callback functions may be different for different callback points.
There are 5 types of callback functions.

- HandlersAccept: Handler for processing connection estalishment
- HandlersRequest: Handler for processing request received
- HandlersForward: Handler for request forwarding process
- HandlersResponse: Handler for processing response received
- HandlersFinish: Handler for processing connection close

The types of callback function are defined in [/bfe_module/bfe_handler_list.go](https://github.com/bfenetworks/bfe/tree/master/bfe_module/bfe_handler_list.go)

The following describes each type of callback functions in detail

Note: For the meaning of type int in the return value below, please refer to "Return Value of Callback Function" section above.

### HandlersAccept

- Applicable callback points:
    + HandleAccept
    + HandleHandshake
- Function prototype:
    + `handler(session *bfe_basic.Session) int`

### HandlersRequest

- Applicable callback point:
    + HandleBeforeLocation
    + HandleFoundProduct
    + HandleAfterLocation
- Function prototype:
    + `handler(req *bfe_basic.Request) (int, *bfe_http.Response)`

### HandlersForward

- Applicable callback point:
    + HandleForward
- Function prototype:
    + `handler(req *bfe_basic.Request) int`

### HandlersResponse

- Applicable callback point:
    + HandleReadResponse
    + HandleRequestFinish
- Function prototype:
    + `handler(req *bfe_basic.Request, res *bfe_http.Response) int`

### HandlersFinish

- Applicable callback point:
    + HandleFinish
- Function prototype:
    + `handler(session *bfe_basic.Session) int`
