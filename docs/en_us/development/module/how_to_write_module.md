# How to Write a Module

## Overview

When writing a module for BFE, the following aspects should be considered:

- How to load configuration
- How to write and register callback functions
- How to expose internal states

mod_block is used as an example for ease of understanding.（[/bfe_modules/mod_block](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block)）

## Load Configuration

### Types of configuration

For a given module, there are 2 types of configuration:

- Static configuration: be loaded when BFE starts
    + There is only one such configuration file for each module
    + The name of the configuration file is the same as the module name. It is suffixed with .conf
    + Example: mod_block.conf

- Dynamic configuration: be loaded when BFE starts. It can also be hot-reloaded without restarting BFE.
    + There can be one or more such configuration files for each module
    + The name of the configuration file usually ends with .data
    + Example: block_rules.data and ip_blocklist.data in mod_block

### Placement of configuration files

- The configuration files of the modules should be placed in [/conf/{module_name}](https://github.com/bfenetworks/bfe/tree/master/conf)
- Example: Configuration files of mod_block are located in [/conf/mod_block](https://github.com/bfenetworks/bfe/tree/master/conf/mod_block)

### Verification of configuration

Configuration files are verified whenever they are loaded, regardless of static or dynamic configuration.

- BFE fails to start if the configuration files fails to be loaded.
- BFE will continue to run if dynamic configuration fails to be hot-reloaded.

### Hot-reload of dynamic configuration

For dynamic configurations, it is required to register callback function on dedicated web server. Hot-reload of dynamic configuration can be triggered by accessing specified URL.

Example: In the init function of mod_block, there is some logic as follows, used to register callback function for configuration reload([mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))

```golang
    // register web handler for reload
    err = whs.RegisterHandler(web_monitor.WebHandleReload, m.name, m.loadConfData)
    if err != nil {
      ...
    }
```

## Write and register callback functions

### Write callback functions

Write callback functions for appropriate callback point.

Note that for different callback points, definition of callback functions may be different. Definition of callback points and  callback functions in BFE can be found in [bfe_callback](./bfe_callback.md).

Example: there are two callback functions defined in mod_block([mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))

```golang
func (m *ModuleBlock) globalBlockHandler(session *bfe_basic.Session) int {
    ...
}

func (m *ModuleBlock) productBlockHandler(request *bfe_basic.Request) (int, *bfe_http.Response) {
    ...
}

```

### Register callback functions

Callback functions should be registered when the module is initialized.

Example: registration of callback functions in mod_block is as follows([mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))

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

## Expose module internal states

For each BFE module, it is strongly recommended to expose enough internal states.

To expose internal states of a module, do the following 3 steps:

- Define state variables
- Register callback function for exposing internal states
- Insert code for doing statistic

### Define state variables

Firstly, design statistical metrics and define them as member variables.

Example: define ModuleBlockState in mod_block ([mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))

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

Secondly, define a member variable of type ModuleBlockState in ModuleBlock. Also define a member variable of type Metrics for related calculations.

```golang
type ModuleBlock struct {
    ...
    state   ModuleBlockState // module state
    metrics metrics.Metrics
    ...
```

Thirdly, do initialization in constructor function.

```golang
func NewModuleBlock() *ModuleBlock {
    m := new(ModuleBlock)
    m.name = ModBlock
    m.metrics.Init(&m.state, ModBlock, 0)
    ...
}
```

### Register a callback function that exposes the internal state

In order to expose internal status, callback function should be implemented.

Example: In mod_block, there is logic as follows,  where monitorHandlers () is the callback function([mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go))

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

Then register callback function during module initialization.

```golang
    // register web handler for monitor
    err = web_monitor.RegisterHandlers(whs, web_monitor.WebHandleMonitor, m.monitorHandlers())
    if err != nil {
        ...
    }
```

### Insert code for statistic

Insert some code for doing statistic.

Example: [mod_block.go](https://github.com/bfenetworks/bfe/tree/master/bfe_modules/mod_block/mod_block.go)

```golang
func (m *ModuleBlock) globalBlockHandler(session *bfe_basic.Session) int {
    ...
    m.state.ConnTotal.Inc(1)
    ...
}
```
