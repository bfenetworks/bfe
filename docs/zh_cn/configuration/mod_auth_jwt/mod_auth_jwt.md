# 简介

mod_auth_jwt支持JWT认证。
  
# 配置说明

## 配置描述
模块配置文件：conf/mod_auth_jwt/mod_auth_jwt.conf

## 配置示例

```
[basic]
# The path of the JWK file 
# For more details, see https://tools.ietf.org/html/rfc7517
SecretPath = mod_auth_jwt/secret.jwk

# Config path for products
ProductConfigPath = mod_auth_jwt/product_config.data

# By default, the module read JWT claims from payload(JWS) or plaintext(JWE) only.
# By setting EnabledHeaderClaims to true, the module will try to read JWT claims from header -
# in the case that a claim validation was enabled while it's not exists in payload(JWS) or plaintext(JWE).
EnabledHeaderClaims = false

# Enabled validation for nested JWT
# NOTICE:This step will be skipped in the case that the nested JWT parse failed,
# it is in the consideration of different encryption key may be used for the nested JWT.
# The nested JWT should be Base64URL-encoded as the payload for JWS,
# or Base64URL-encoded as encrypted plaintext for JWE.
ValidateNested = true

# Validation for JWT claims
# NOTICE: Validation for claims will be applied when relative claim(s) present in the JWT header -
# or payload(when EnabledPayloadClaims was set to true). When no any relative claim(s) present, -
# only basic validation (for example: signature check) will be applied.
# For more details, see https://tools.ietf.org/html/rfc7519#section-4

# Enable validation for Expiration claim
ValidateClaimExp = true

# Enable validation for Not Before claim
ValidateClaimNbf = true

# Enable validation for Issuer claim
# ValidateClaimIss = issuer

# Enable validation for Subject claim
# ValidateClaimSub = subject

# Enable validation for Audience claim
# ValidateClaimAud = audience

[log]
OpenDebug = true
```

# 规则配置
## 配置示例
```
{
	"Version": "Version",
	"Config": {
                "example_product": [
                        "Cond": "req_host_in(\"www.example.org\")",
		}
	}
}
```
  
# 使用约定
用法：需将token放置到HTTP请求Authorization头中，并指定为Bearer类型。
示例：`Authorization: Bearer <token>`

JWT(包括嵌套JWT)必须使用压缩序列格式(Compact Serialization)，不支持JSON序列格式(JSON Serialization)。具体原因见下述用法说明。
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

