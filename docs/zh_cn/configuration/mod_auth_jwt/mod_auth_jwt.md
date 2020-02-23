# 简介

mod_auth_jwt支持JWT令牌(JWS和JWE)身份认证。

# 使用说明

需要将token放置到HTTP请求的Authorization请求头中，并指定为Bearer类型。

示例：`Authorization: Bearer <token>`

+ 模块配置：

  模块配置必须配置字段：SecretPath、ProductConfigPath。

  **SecretPath**是存放认证私钥的文件路径，它是一个JSON格式的键-值对配置文件，有关键-值对的信息，请参阅JWK：https://tools.ietf.org/html/rfc7517。

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
  	"k": "secret key", // 密钥
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
  
  # By default, the module read JWT claims from header only.
  # By setting EnabledPayloadClaims to true, the module will try to read JWT claims from payload -
  # in the case that a claim validation was enabled while it's not exists in the JWT header (Only effective for JWS, NO JWE).
  # Can be override in product config
  EnabledPayloadClaims = false
  
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

  