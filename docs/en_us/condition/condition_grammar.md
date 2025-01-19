# Concept and Grammar

## Basic Concepts

### Condition Primitive

- A condition primitive is a basic and built-in function which check some specified condition matched or not.

``` go
// return true if the request Host is "www.bfe-networks.com" or "bfe-networks.com"
req_host_in("www.bfe-networks.com|bfe-networks.com")
```

- BFE provides a set of built-in [condition primitives](condition_primitive_index.md)

### Condition Expression

- A condition expression is a series of condition primitives combined with operators (e.g. AND, OR, NOT, etc).

```go
// return ture if the request host is "bfe-networks.com" and the request method is "GET"
req_host_in("bfe-networks.com") && req_method_in("GET")
```

* The supported operators are described in another section below.

### Condition Variable

- You can define a variable  and assign a condition expression to it.

```go
// define a condition variable
bfe_host = req_host_in("bfe-networks.com")
```

### Advanced Condition Expression

- An advanced condition expression is a series of condition primitives and  condition variables combined with  operators (e.g. AND, OR, NOT, etc).

- In an advanced condition expression, the condition variable is identified by  "$" prefix

```go
// return true if the value of new_host is true and the request method is GET
$new_host && req_method_in("GET")
```

## Grammar

### Condition Primitive Grammar

A condition primitive is shown as follows:

```go
func_name(params)
```

- **func_name** is the name of condition primitive
- **params** are the  parameters condition primitive
- The type of return value is **bool**

### Condition Expression Grammar

Condition Expression(CE) grammar is defined as follows:

```
CE = CE && CE
   | CE || CE
   | ( CE )
   | ! CE
   | ConditionPrimitive
```

### Advanced Condition Expression Grammar

Advanced Condition Expression(ACE) grammar is defined as follows:

```
ACE = ACE && ACE
    | ACE || ACE
    | ( ACE)
    | ! ACE
    | ConditionPrimitive
    | ConditionVariable
```

### Operator Precedence

The precedence and associativity of operators are similar to the C language. The following table lists the precedence and associativity of all operators. Operators are listed top to bottom, in descending precedence.

| Precedence | Operator | Description            | Associativity |
| ---------- | -------- | ---------------------- | ------------- |
| 1          | ()       | parentheses (grouping) | Left-to-right |
| 2          | !        | logical NOT            | Right-to-left |
| 3          | &&       | logical AND            | Left-to-right |
| 4          | \|\|     | logical OR             | Left-to-right |
