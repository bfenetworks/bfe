# 概念及语法

## 基本概念

### 条件原语

- 条件原语是基本的内置条件判断单元，执行某种比较来判断是否满足条件

``` go
// 如果请求host是"bfe-networks.com"或"bfe-networks.org", 返回true
req_host_in("bfe-networks.com|bfe-networks.org") 
```

- BFE支持一系列预定义的内置[条件原语](condition_primitive_index.md)

### 条件表达式

- 条件表达式是多个条件原语与操作符(例如与、或、非)的组合

```go
// 如果请求域名是"bfe-networks.com"且请求方法是"GET", 返回true
req_host_in("bfe-networks.com") && req_method_in("GET") 
```

* 支持的操作符详见下文说明

### 条件变量

- 可以将条件表达式赋值给一个变量，这个变量被定义为条件变量

```go
// 将条件表达式赋值给变量bfe_host
bfe_host = req_host_in("bfe-networks.com") 
```

### 高级条件表达式

- 高级条件表达式是多个条件原语和条件变量与操作符(例如与、或、非)的组合

- 在高级条件表达式中，条件变量以$前缀作为标示

```go
// 如果变量bfe_host为true且请求方法是"GET"，返回true
$bfe_host && req_method_in("GET") 
```

## 语法说明

### 条件原语的语法

条件原语的形式如下：

```go
func_name(params)
```

- **func_name**是条件原语名称
- **params**是条件原语的参数，可能是0个或多个
- 返回值类型是**bool**

### 条件表达式的语法

条件表达式(CE: Condition Expression)的语法定义如下：

```
CE = CE && CE
   | CE || CE
   | ( CE )
   | ! CE
   | ConditionPrimitive
```

### 高级条件表达式的语法

高级条件表达式(ACE: Advanced Condition Expression)的语法定义如下：

```
ACE = ACE && ACE
    | ACE || ACE
    | ( ACE )
    | ! ACE
    | ConditionPrimitive
    | ConditionVariable
```

### 操作符优先级

操作符的优先级和结合律与C语言中类似。下表列出了所有操作符的优先级及结合律。操作符从上至下按操作符优先级降序排列。

| 优先级 | 操作符 | 含义   | 结合律   |
| ------ | ------ | ------ | -------- |
| 1      | ()     | 括号   | 从左至右 |
| 2      | !      | 逻辑非 | 从右至左 |
| 3      | &&     | 逻辑与 | 从左至右 |
| 4      | \|\|   | 逻辑或 | 从左至右 |
