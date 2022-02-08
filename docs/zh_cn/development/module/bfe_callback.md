# BFE的回调机制

## BFE的转发过程和回调点

BFE转发过程中的回调点如下图所示。
![BFE转发过程中的回调点](../../../images/bfe-callback.png)

## 回调点列表

在BFE中，设置了以下9个回调点：

- HandleAccept: 位于和客户端的TCP连接建立后
- HandleHandshake：位于和客户端的SSL或TLS握手完成后
- HandleBeforeLocation：位于查找产品线之前
- HandleFoundProduct：位于完成查找产品线之后
- HandleAfterLocation：位于完成查找集群之后
- HandleForward：位于完成查找子集群和后端实例之后，位于转发请求之前
- HandleReadResponse：位于读取到后端响应之后
- HandleRequestFinish：位于后端响应处理完毕后
- HandleFinish：位于和客户端的TCP连接关闭后

回调点的定义，可以查看[/bfe_module/bfe_callback.go](https://github.com/bfenetworks/bfe/tree/master/bfe_module/bfe_callback.go)

## 回调函数的返回值

在回调点执行由模块注册的回调函数，回调函数会返回特定的返回值。BFE根据回调函数的返回值，执行后续的操作。

回调函数的返回值，定义如下：

- BfeHandlerFinish：在发送响应后，关闭连接
- BfeHandlerGoOn：继续执行下一个回调函数
- BfeHandlerRedirect：执行重定向（Redirect）
- BfeHandlerResponse：发送响应
- BfeHandlerClose：直接关闭连接，不发送任何数据

回调函数返回值的定义，可以查看[/bfe_module/bfe_handler_list.go](https://github.com/bfenetworks/bfe/tree/master/bfe_module/bfe_handler_list.go)

## 回调函数的形式

在不同的回调点，回调函数的形式也是不同的。在BFE中，定义了以下5种类型的回调函数

- HandlersAccept：用于处理连接建立的相关场景
- HandlersRequest：用于处理和请求有关的场景
- HandlersForward：用于处理和转发有关的场景
- HandlersResponse：用于处理和响应有关的场景
- HandlersFinish：用于处理连接关闭的相关场景

回调函数类型的定义，可以查看[/bfe_module/bfe_handler_list.go](https://github.com/bfenetworks/bfe/tree/master/bfe_module/bfe_handler_list.go)

下面对这几种回调函数做详细的说明。

注：下面回调函数中int类型的返回值，参见上述“回调函数的返回值”中的说明。

### HandlersAccept

- 适用回调点：
    + HandleAccept
    + HandleHandshake
- 回调函数形式：
    + `handler(session *bfe_basic.Session) int`

### HandlersRequest

- 适用回调点：
    + HandleBeforeLocation
    + HandleFoundProduct
    + HandleAfterLocation
- 回调函数形式：
    + `handler(req *bfe_basic.Request) (int, *bfe_http.Response)`

### HandlersForward

- 适用回调点：
    + HandleForward
- 回调函数形式：
    + `handler(req *bfe_basic.Request) int`

### HandlersResponse

- 适用回调点：
    + HandleReadResponse
    + HandleRequestFinish
- 回调函数形式：
    + `handler(req *bfe_basic.Request, res *bfe_http.Response) int`

### HandlersFinish

- 适用回调点：
    + HandleFinish
- 回调函数形式：
    + `handler(session *bfe_basic.Session) int`
