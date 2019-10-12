# Condition Grammar

## Basic Concepts

- **Condition Primitive**：

  - Basic conditional judgment unit, which defines the primitive of comparison;

  - e.g.

    ``` 
    req_host_in(“www.bfe-networks.com|bfe-networks.com”)  # host is one of the configured domains
    ```

- **Condition Expression**

  - Expression using "and/or/not" to connect condition primitive ；

  - e.g.

    ```
    req_host_in(“bfe-networks.com”) && req_method_in(“GET”) # domain is bfe-networks.com and HTTP method is "GET"
    ```

- **Condition Variable**

  - Variable that is defined by **Condition Expression**;

  - e.g.

    ```
    bfe_host = req_host_in(“bfe-networks.com”)  # variable bfe_host is defined by condition expression 
    ```

- **Advanced Condition Expression**

  - Expression using "and/or/not" to connect condition primitive and condition variable

  - In advanced condition expression, condition variable is identified by  **"$" prefix**；

  - e.g.
  
    ```
    $news_host && req_method_in("GET") # match condition variable and HTTP method is "GET"
    ```


## Condition Primitive Grammar

- Basic conditional judgment unit, format is shown as fol：

​           **FuncName( params )**

-  The number of parameters can be one or more

- Condition Primitive Grammar
  - The type of returning  value is bool；
  - Condition primitive like function definition, multiple parameters are supported in it;


## Condition Expression Grammar

Condition Expression grammar is defined as follows:

- Priority and combination rule of "&&/||/!" is same as them in C language;

- Expression description

  ```
  Condition Expression(CE) -> 
  CE && CE
                   | CE || CE
                   | ( CE )
                   | ! CE
                   | Condition Primitive
  ```
  
  

## Advanced Condition Expression Grammar

Advanced Condition Expression grammar is defined as follows:：

- Priority and combination rule of "&&/||/!" is same as them in C language;

- Expression description

  ```
  Advanced Condition Expression(ACE) -> 
  ACE && ACE
                   | ACE || ACE
                   | ( ACE)
                   | ! ACE
                   | Condition Primitive
  | Condition Variable
  ```
  
  
