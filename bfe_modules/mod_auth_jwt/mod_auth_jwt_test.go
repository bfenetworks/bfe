package mod_auth_jwt

import (
	"fmt"
	"github.com/baidu/bfe/bfe_basic"
	"github.com/baidu/bfe/bfe_http"
	"github.com/baidu/bfe/bfe_module"
	"github.com/baidu/go-lib/web-monitor/web_monitor"
	"io/ioutil"
	"os"
	"testing"
)

var (
	request = new(bfe_basic.Request)
	module  = NewModuleAuthJWT()
)

func init() {
	httpRequest, err := bfe_http.NewRequest("GET", "http://www.example.org", nil)
	if err != nil {
		panic(err)
	}
	request.HttpRequest = httpRequest
	request.Session = new(bfe_basic.Session)
	callbacks := bfe_module.NewBfeCallbacks()
	handlers := web_monitor.NewWebHandlers()
	confRoot := "./testdata"
	err = module.Init(callbacks, handlers, confRoot)
	if err != nil {
		panic(err)
	}
}

func TestAuthService_valid(t *testing.T) {
	products := []string{"jwe_valid_1", "jws_valid_1"}
	for _, product := range products {
		request.Route.Product = fmt.Sprintf("test_%s", product)
		file, err := os.Open(fmt.Sprintf("./testdata/mod_auth_jwt/%s.txt", product))
		if err != nil {
			t.Fatal(err)
		}
		token, _ := ioutil.ReadAll(file)
		_ = file.Close()
		t.Logf("%s", token)
		request.HttpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		flag, response := module.authService(request)
		if flag != bfe_module.BfeHandlerGoOn {
			t.Logf("%+v", response)
			t.Errorf("Expected flag code %d, got %d", bfe_module.BfeHandlerGoOn, flag)
			return
		}
	}
}

//func TestAuthService_invalid(t *testing.T) {
//	products := []string{"jwe_invalid_1", "jws_invalid_1",}
//	for _, product := range products {
//		request.Route.Product = fmt.Sprintf("test_%s", product)
//		file, err := os.Open(fmt.Sprintf("./testdata/mod_auth_jwt/%s.txt", product))
//		if err != nil {
//			t.Fatal(err)
//		}
//		token, _ := ioutil.ReadAll(file)
//		_ = file.Close()
//		t.Logf("%s", token)
//		request.HttpRequest.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
//		flag, response := module.authService(request)
//		if flag != bfe_module.BfeHandlerResponse {
//			t.Errorf("Expected flag code %d, got %d", bfe_module.BfeHandlerResponse, flag)
//			return
//		}
//		t.Logf("%+v", response)
//	}
//}
