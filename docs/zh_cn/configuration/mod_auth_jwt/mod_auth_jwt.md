# 简介

mod_auth_jwt支持JWT令牌(JWS和JWE)基础验证、字段验证、嵌套JWT验证。

+ 支持通过配置进行验证的字段：
  + exp (Expiration)
  + nbf (Not Before)
  + iss (Issuer)
  + aud (Audience)
  + sub (Subject)

+ alg和enc的支持情况：
  + alg for JWS：
    - none - **不推荐**
    - HS256/384/512
    - RS256/384/512
    - ES256/384/512
    - PS256/384/512
  + alg for JWE：
    + dir
    + RSA1_5
    + RSA-OAEP / RSA-OAEP-256
    + A128KW / A192KW / A256KW
    + A128GCMKW / A192GCMKW / A256GCMKW
    + ECDH-ES
    + ECDH-ES+A128KW / ECDH-ES+A192KW / ECDH-ES+A256KW
    + PBES2-HS256+A128KW / PBES2-HS384+A192KW / PBES2-HS512+A256KW
    + **注：使用PBES系列算法请将passphrase进行base64URL编码后以对称密钥的形式在私钥文件内提供。**
  + enc for JWE：
    + A128CBC-HS256 / A192CBC-HS384 / A256CBC-HS512
    + A128GCM / A192GCM / A256GCM

# 使用说明

+ 一些约定：

  - JWT(包括嵌套JWT)必须使用压缩序列格式(Compact Serialization)，不支持JSON序列格式(JSON Serialization)。具体原因见下述用法说明。

  - 关于嵌套JWT

    * 在JWS中，将进行了base64URL编码后的JWT直接作为JWS的Payload部分，则Payload为：base64URLEncode(Nested JWT)。
    * 在JWE中，将进行了base64URL编码后的JWT作为JWE的plaintext，则ciphertext为：base64URLEncode(Encrypt(Nested JWT))。
    * 当嵌套JWT能够被正常解析(未解密，仅解析)时，JWT验证通过的条件为**主体JWT验证通过+嵌套JWT验证通过**(这是个递归过程)。
    * 对于无法正常解析的嵌套JWT(密钥不同或者格式问题)，对嵌套JWT的验证会被跳过(默认成功)，此时JWT验证通过的条件仅为**主体JWT验证通过**。

  - 关于基础验证

    * 对于JWS，若签名校验通过，则认为基础验证通过。
    * 对于JWE，若**能够正确解密出CEK(Content Encrypted Key)**、且利用该CEK**能够解密出明文内容**，即解密明文过程无异常抛出(无视明文具体内容，事实上对于明文具体内容也没有一个标准能够进行检查)，则认为基础验证通过。

  - 关于字段验证

    * 开启字段验证仅在能够在JWT中查找到相关字段的条件下生效，当字段不存在时，相关字段验证会被跳过(默认成功)。

    * 默认的，进行字段验证时仅在JWS的Payload部分或JWE的plaintext部分查找相关字段，可以通过配置启用EnabledHeaderClaims配置项达到当默认位置不存在相应字段时，则到Header部分中查找相关字段的目的。JWS的签名验证机制天然地可以保证Header和Payload的数据完整性；JWE虽然没有签名验证机制，但由于其解密AAD与Header相关，Header被篡改会导致AAD不正确，无法正确解密出明文，间接保证了JWE的Header的数据完整性。因此，启用该配置选项不必担心数据完整性问题，无论是对于JWS或是JWE。

    * 由于[JWE](https://tools.ietf.org/html/rfc7516)的RFC文档中并未规定plaintext的具体格式；而[JWT](https://tools.ietf.org/html/rfc7519#section-3)的RFC文档说明了在JWE中，声明字段应为JWE的plaintext部分。故约定，对于JWE，在需要承载声明字段的情况下，将进行了base64URL编码后的声明字段JSON字符串作为JWE的plaintext，则ciphertext为：base64URLEncode(Encrypt(Claim Set))。在plaintext无法应用该规则进行解码、且未启用EnabledHeaderClaims的情况下，对JWE的字段验证会被跳过(默认成功)。

用法：需要将token放置到HTTP请求的Authorization请求头中，并指定为Bearer类型。

示例：`Authorization: Bearer <token>`

+ 模块配置：

  模块配置必须配置字段：SecretPath、ProductConfigPath。

  **SecretPath**是存放认证私钥的文件路径，它是一个JSON格式的键-值对配置文件，有关键-值对的信息，请参阅：

  + JWK：https://tools.ietf.org/html/rfc7517
  + JWA：https://tools.ietf.org/html/rfc7518

  **ProductConfigPath**是存放对产品相关规则配置的文件路径，同样是一个JSON格式的键-值对配置文件，更多信息参阅下述配置规则。

+ 配置实例(HS256):

  模块配置：

  ```
  [basic]
  SecretPath = ./secret.key
  ProductConfigPath = ./product.conf
  // ...
  ```

  secret.key：

  ```
  {
  	“kty": "oct", // 密钥类型:字节序列(对称密钥)
  	"k": "...", // base64URL编码的对称密钥
  	// ...
  }
  ```

  product.conf：

  ```
  {
  	"Version": "Version",
  	"Config": {
  		"test": {
  			"Cond": "req_host_in(\"www.example.org\")", // 命中条件
  			// ...
  		},
  		// ...
  	}
  }
  ```

  

# 配置说明

- 模块配置文件：conf/mod_auth_jwt/mod_auth_jwt.conf

  ```
  [basic]
  # The path of the file saving secret key
  # A key-value mapping JSON type file
  # for more key-value information(JWK): https://tools.ietf.org/html/rfc7517
  # Can be override in product config
  SecretPath = ./SECRET.key
  # Config path for products
  ProductConfigPath = ./product_config.data
  
  # By default, the module read JWT claims from payload(JWS) or plaintext(JWE) only.
  # By setting EnabledHeaderClaims to true, the module will try to read JWT claims from header -
  # in the case that a claim validation was enabled while it's not exists in payload(JWS) or plaintext(JWE).
  # Can be override in product config
  EnabledHeaderClaims = false
  
  # Enabled validation for nested JWT
  # NOTICE:
  # This step will be skipped in the case that the nested JWT parse failed,
  # it is in the consideration of different encryption key may be used for the nested JWT.
  # The nested JWT should be Base64URL-encoded as the payload for JWS,
  # or Base64URL-encoded as encrypted plaintext for JWE.
  ValidateNested = true
  
  # Validation for JWT claims
  # NOTICE:
  # Validation for claims will be applied when relative claim(s) present in the JWT header -
  # or payload(when EnabledPayloadClaims was set to true). When no any relative claim(s) present, -
  # only basic validation (for example: signature check) will be applied.
  # All claims validation can be override in product config
  # For more claims detail: https://tools.ietf.org/html/rfc7519#section-4
  
  # exp (bool)
  ValidateClaimExp = true
  # nbf (bool)
  ValidateClaimNbf = true
  # iss (string)
  # ValidateClaimIss = issuer
  # sub (string)
  # ValidateClaimSub = subject
  # aud (string)
  # ValidateClaimAud = audience
  
  [log]
  OpenDebug = true
  
  ```

- 产品配置文件格式：

  ```
  {
  	"Version": "Version",
  	"Config": {
  		"产品名": {
  			"Cond": "", // 命中条件
  			// 其他配置用于覆盖上述模块配置(override)
  		}
  	}
  }
  ```

  