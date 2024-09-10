# 如何编写一个模块

## 概述

在为BFE编写一个新的模块时，需要考虑以下方面：

- 配置的加载
- 回调函数的编写
- 模块状态的展示

在下面的讲述中，将结合mod_block的实现作为例子。

mod_block的代码位于[/bfe_modules/mod_block](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block)

## 配置的加载

### 配置的分类

对于一个模块，包括两种配置：

- 静态加载的配置：在BFE程序启动的时候加载
    + 对每个模块有一个这样的配置文件
    + 配置文件的名字和模块名字一致，并以.conf结尾
    + 如：mod_block.conf

- 可动态加载的配置：在BFE程序启动的时候加载，在BFE运行过程中也可以动态加载
    + 对每个模块可以有一个或多个这样的配置文件
    + 配置文件的名字一般以.data结尾
    + 如：在mod_block下有 block_rules.data 和 ip_blocklist.data

对于每一个配置文件，应编写独立的加载逻辑。

### 配置文件的放置

- 模块的配置文件，应该统一放置于[/conf](https://github.com/bfenetworks/bfe/tree/master/conf)目录下为每个模块独立建立的目录中
- 如：mod_block的配置文件，都放置在[/conf/mod_block](https://github.com/bfenetworks/bfe/tree/master/conf/mod_block)中

### 配置加载的检查

无论对于静态加载的配置，还是对于可动态加载的配置，为了保证程序正常的运行，在配置加载的时候，都需要对于配置文件的正确性进行检查。

- 在BFE程序启动阶段，如果模块的配置文件加载失败，则BFE无法启动
- 在BFE程序启动后，如果模块的可动态加载配置文件加载失败，BFE仍然会继续运行

### 配置的动态加载

对于可动态加载的配置，需要在BFE用于监控和加载的专用web server上做回调注册。这样，通过访问BFE对外监控端口的特定URL，就可以触发某个配置的动态加载。

例如，在mod_block的初始化函数中，可以看到类似下面的逻辑，就是在注册配置加载的回调(详见[mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go)):

```golang
    // register web handler for reload
    err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
    if err != nil {
      ...
    }
```

## 回调函数的编写和注册

### 回调函数的编写

根据模块的功能需求，选择合适的回调点，对应编写回调函数。

注意，对于不同的回调点，回调函数的形式可能不同。BFE所提供的回调点和回调函数的形式，可参考[BFE的回调机制](./bfe_callback.md)

例如，在mod_block中，编写了以下两个回调函数(详见[mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))：

```golang
func (m *ModuleBlock) globalBlockHandler(session *bfe_basic.Session) int {
    ...
}

func (m *ModuleBlock) productBlockHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
    ...
}

```

### 回调函数的注册

为了让回调函数生效，需要在模块初始化的时候对回调函数进行注册。

例如，在mod_block中，回调函数的注册逻辑如下(详见[mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))：

```golang
func (m *ModuleBlock) Init(cbs *bfe_module.BfeCallbacks, whs *web_monitor.WebHandlers, cr string) error {
    ...
    // register handler
    err = cbs.AddFilter(bfe_module.HandleAccept, m.globalBlockHandler)
    if err != nil {
        ...
    }
    
    err = cbs.AddFilter(bfe_module.HandleFoundProduct, m.productBlockHandler)
    if err != nil {
        ...
    }
    ...
}
```

## 模块状态的展示

对于每个BFE的模块，强烈推荐通过BFE规定的机制，向外暴露足够的内部状态信息。

在模块对外暴露内部状态，需要做以下3步：

- 定义状态变量
- 注册显示内部状态的回调函数
- 在代码中插入状态设置逻辑

### 定义状态变量

需要首先设计在模块中需要统计哪些值，并通过结构体成员变量的方式定义出来。

如，在mod_block中，有如下定义(详见[mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))：

```golang
type ModuleBlockState struct {
    ConnTotal    *metrics.Counter // all connnetion checked
    ConnAccept   *metrics.Counter // connection passed
    ConnRefuse   *metrics.Counter // connection refused
    ReqTotal     *metrics.Counter // all request in
    ReqAccept    *metrics.Counter // request accepted
    ReqRefuse    *metrics.Counter // request refused
    WrongCommand *metrics.Counter // request with condition satisfied, but wrong command
}
```

然后，要在ModuleBlock中定义一个类型为ModuleBlockState的成员变量，还需要定义一个Metrics类型的成员变量，用于相关的计算。

```golang
type ModuleBlock struct {
    ...
    state   ModuleBlockState // module state
    metrics metrics.Metrics
    ...
```

然后，需要在构造函数中做初始化的操作

```golang
func NewModuleBlock() *ModuleBlock {
    m := new(ModuleBlock)
    m.name = ModBlock
    m.metrics.Init(&m.state, ModBlock, 0)
    ...
}
```

### 注册显示内部状态的回调函数

为了可以通过BFE的监控端口查看模块的内部状态，需要首先实现回调函数。

如，在mod_block中，有如下逻辑(详见[mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))，其中monitorHandlers()是回调函数：

```golang
func (m *ModuleBlock) getState(params map[string][]string) ([]byte, error) {
    s := m.metrics.GetAll()
    return s.Format(params)
}

func (m *ModuleBlock) getStateDiff(params map[string][]string) ([]byte, error) {
    s := m.metrics.GetDiff()
    return s.Format(params)
}

func (m *ModuleBlock) monitorHandlers() map[string]interface{} {
    handlers := map[string]interface{}{
        m.name:           m.getState,
        m.name + ".diff": m.getStateDiff,
    }
    return handlers
}
```

然后，在模块的初始化时，需要注册这个回调函数

```golang
    // register web handler for monitor
    err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
    if err != nil {
        ...
    }
```

### 在代码中插入统计逻辑

在模块的执行逻辑中，可以插入一些统计的代码。

如，在mod_block中，可以看到如下代码(详见[mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))：

```golang
func (m *ModuleBlock) globalBlockHandler(session *bfe_basic.Session) int {
    ...
    m.state.ConnTotal.Inc(1)
    ...
}
```
