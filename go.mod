module github.com/bfenetworks/bfe

go 1.21

toolchain go1.22.2

require (
	github.com/abbot/go-http-auth v0.4.1-0.20181019201920-860ed7f246ff
	github.com/andybalholm/brotli v1.0.2
	github.com/armon/go-radix v1.0.0
	github.com/asergeyev/nradix v0.0.0-20170505151046-3872ab85bb56 // indirect
	github.com/baidu/go-lib v0.0.0-20200819072111-21df249f5e6a
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/gomodule/redigo v2.0.0+incompatible
	github.com/json-iterator/go v1.1.12
	github.com/microcosm-cc/bluemonday v1.0.16
	github.com/miekg/dns v1.1.29
	github.com/opentracing/opentracing-go v1.1.0
	github.com/openzipkin-contrib/zipkin-go-opentracing v0.4.5
	github.com/openzipkin/zipkin-go v0.2.2
	github.com/oschwald/geoip2-golang v1.4.0
	github.com/pkg/errors v0.9.1 // indirect
	github.com/russross/blackfriday/v2 v2.0.1
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	github.com/spaolacci/murmur3 v1.1.0
	github.com/stretchr/testify v1.9.0
	github.com/tjfoc/gmsm v1.3.2
	github.com/uber/jaeger-client-go v2.25.0+incompatible
	github.com/uber/jaeger-lib v2.4.0+incompatible
	github.com/zmap/go-iptree v0.0.0-20170831022036-1948b1097e25
	go.elastic.co/apm v1.11.0
	go.elastic.co/apm/module/apmot v1.7.2
	go.uber.org/atomic v1.7.0 // indirect
	go.uber.org/automaxprocs v1.4.0
	golang.org/x/crypto v0.25.0
	golang.org/x/net v0.25.0
	golang.org/x/sys v0.22.0
	gopkg.in/gcfg.v1 v1.2.3
	gopkg.in/warnings.v0 v0.1.2 // indirect
)

require (
	github.com/bfenetworks/proxy-wasm-go-host v0.0.0-20241202144118-62704e5df808
	github.com/go-jose/go-jose/v4 v4.0.4
)

require (
	github.com/HdrHistogram/hdrhistogram-go v1.0.1 // indirect
	github.com/aymerick/douceur v0.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/elastic/go-sysinfo v1.1.1 // indirect
	github.com/elastic/go-windows v1.0.0 // indirect
	github.com/gorilla/css v1.0.0 // indirect
	github.com/jehiah/go-strftime v0.0.0-20171201141054-1d33003b3869 // indirect
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/opentracing-contrib/go-observer v0.0.0-20170622124052-a52f23424492 // indirect
	github.com/oschwald/maxminddb-golang v1.6.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/procfs v0.2.0 // indirect
	github.com/santhosh-tekuri/jsonschema v1.2.4 // indirect
	github.com/tetratelabs/wazero v1.2.1 // indirect
	go.elastic.co/apm/module/apmhttp v1.7.2 // indirect
	go.elastic.co/fastjson v1.1.0 // indirect
<<<<<<< HEAD
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/grpc v1.56.3 // indirect
=======
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/grpc v1.22.1 // indirect
>>>>>>> master
	gopkg.in/yaml.v3 v3.0.1 // indirect
	howett.net/plist v0.0.0-20181124034731-591f970eefbb // indirect
)

// replace github.com/bfenetworks/proxy-wasm-go-host => ../proxy-wasm-go-host
