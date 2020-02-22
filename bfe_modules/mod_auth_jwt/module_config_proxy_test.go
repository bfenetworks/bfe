package mod_auth_jwt

import "testing"

func TestNewModuleConfigProxy_GetWithLock(t *testing.T) {
	config, err := NewModuleConfigProxy("./testdata/mod_auth_jwt/mod_auth_jwt.conf")
	if err != nil {
		t.Error(err)
		return
	}
	v, ok := config.GetWithLock("Basic.SecretPath")
	if !ok {
		t.Error("Failed to get field from ModuleConfigProxy with lock")
		return
	}
	expected := config.Config.Basic.SecretPath
	if v.String() != expected {
		t.Errorf("Expected %s, got %+v", expected, v)
	}
}

func TestModuleConfigProxy_FindProductConfig(t *testing.T) {
	config, err := NewModuleConfigProxy("./testdata/mod_auth_jwt/mod_auth_jwt.conf")
	if err != nil {
		t.Error(err)
		return
	}
	_, ok := config.FindProductConfig("test")
	if !ok {
		t.Error("Unexpected failed to get product config")
		return
	}
}

func TestModuleConfigProxy_Update(t *testing.T) {
	config, err := NewModuleConfigProxy("./testdata/mod_auth_jwt/mod_auth_jwt.conf")
	if err != nil {
		t.Error(err)
		return
	}
	rawConfig, rawProductConfig := config.Config, config.ProductConfig
	err = config.Update("./testdata/mod_auth_jwt/mod_auth_jwt.conf") // simply reload config
	if err != nil {
		t.Error(err)
		return
	}
	if !(rawConfig != config.Config && rawProductConfig != config.ProductConfig) {
		t.Error("Maybe module config reload failed")
	}
}
