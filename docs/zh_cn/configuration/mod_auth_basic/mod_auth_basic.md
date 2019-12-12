# 简介 

mod_auth_basic支持HTTP基本认证。

# 配置

- 模块配置文件

  conf/mod_auth_basic/mod_auth_basic.conf

  ```
  [basic]
  DataPath = ../conf/mod_auth_basic/auth_basic_rule.data

  [log]
  OpenDebug = false
  ```

- 规则配置文件

  conf/mod_auth_basic/auth_basic_rule.data

   ```
	{
	    "Config": {
	        "example_product": [
	            {
	                "Cond": "req_host_in(\"www.example.org\")",
	                "UserFile": "../conf/mod_auth_basic/userfile",
	                "Realm": "example_product"
	            }
	        ]
	    },
	    "Version": "20190101000000"
	}
  ```

- 用户密码文件

  密码使用MD5、SHA1或BCrypt进行哈希编码。
  
  可以使用htpasswd、openssl生成userfile文件。
  
  openssl生成密码示例：printf "user1:$(openssl passwd -apr1 123456)\n" >> ./userfile。

  ```
    # user1, 123456
    user1:$apr1$mI7SilJz$CWwYJyYKbhVDNl26sdUSh/

    user2:{SHA}fEqNCco3Yq9h5ZUglD3CZJT4lBs=:user2, 123456
  ```