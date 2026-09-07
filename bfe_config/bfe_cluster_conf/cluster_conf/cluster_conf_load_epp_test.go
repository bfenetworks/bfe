// Copyright (c) 2026 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cluster_conf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func strPtr(s string) *string          { return &s }
func intPtr(i int) *int                { return &i }
func strSlicePtr(s []string) *[]string { return &s }

func eppGslbBasicConf(addrs []string) *GslbBasicConf {
	return &GslbBasicConf{
		CrossRetry:  intPtr(0),
		RetryMax:    intPtr(2),
		HashConf:    &HashConf{},
		BalanceMode: strPtr(BalanceModeEPP),
		EPPAddr:     strSlicePtr(addrs),
	}
}

func TestGslbBasicConfEPPAddrCheck(t *testing.T) {
	t.Run("valid ordered addrs", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002", "10.0.0.2:9002"})
		require.NoError(t, GslbBasicConfCheck(conf))
	})

	t.Run("single addr is valid", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"127.0.0.1:9002"})
		require.NoError(t, GslbBasicConfCheck(conf))
	})

	t.Run("empty addr list rejected", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{})
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("invalid host:port rejected", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1"})
		assert.Error(t, GslbBasicConfCheck(conf))

		conf = eppGslbBasicConf([]string{":9002"})
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("duplicate addrs rejected", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002", "10.0.0.1:9002"})
		assert.Error(t, GslbBasicConfCheck(conf))
	})
}

func TestGslbBasicConfEPPCheckDefaults(t *testing.T) {
	conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
	require.NoError(t, GslbBasicConfCheck(conf))
	require.NotNil(t, conf.EPPCheck)
	assert.False(t, conf.EPPCheck.Disabled)
	assert.Equal(t, "2s", *conf.EPPCheck.CheckInterval)
	assert.Equal(t, 3, *conf.EPPCheck.FailThreshold)
	assert.Equal(t, "45s", *conf.EPPCheck.Cooldown)
	assert.Equal(t, 2, *conf.EPPCheck.SuccessThreshold)
	assert.Equal(t, 2*time.Second, conf.EPPCheck.CheckIntervalDuration())
	assert.Equal(t, 45*time.Second, conf.EPPCheck.CooldownDuration())

	require.NotNil(t, conf.EPPTimeout)
	assert.Equal(t, "500ms", *conf.EPPTimeout.Connect)
	assert.Equal(t, "3s", *conf.EPPTimeout.Call)
	assert.Equal(t, 500*time.Millisecond, conf.EPPTimeout.ConnectDuration())
	assert.Equal(t, 3*time.Second, conf.EPPTimeout.CallDuration())
}

func TestGslbBasicConfEPPCheckInvalid(t *testing.T) {
	t.Run("bad check interval duration", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPCheck = &EPPCheckConf{CheckInterval: strPtr("abc")}
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("bad cooldown duration", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPCheck = &EPPCheckConf{Cooldown: strPtr("10")}
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("non-positive interval", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPCheck = &EPPCheckConf{CheckInterval: strPtr("0s")}
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("fail threshold less than 1", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPCheck = &EPPCheckConf{FailThreshold: intPtr(0)}
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("bad connect timeout", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPTimeout = &EPPTimeoutConf{Connect: strPtr("1x")}
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("bad call timeout", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPTimeout = &EPPTimeoutConf{Call: strPtr("")}
		assert.Error(t, GslbBasicConfCheck(conf))
	})
}

func TestGslbBasicConfEPPTLS(t *testing.T) {
	t.Run("nil EPPTLS keeps legacy behavior", func(t *testing.T) {
		// old cluster_conf.data without EPPTLS must still load
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		require.NoError(t, GslbBasicConfCheck(conf))
		assert.Nil(t, conf.EPPTLS)
	})

	t.Run("insecure without CAFile is valid", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPTLS = &EPPTLSConf{Insecure: true}
		require.NoError(t, GslbBasicConfCheck(conf))
	})

	t.Run("secure without CAFile rejected", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPTLS = &EPPTLSConf{Insecure: false}
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("secure with unreadable CAFile rejected", func(t *testing.T) {
		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPTLS = &EPPTLSConf{Insecure: false, CAFile: "/nonexistent/epp_ca.crt"}
		assert.Error(t, GslbBasicConfCheck(conf))
	})

	t.Run("secure with valid CAFile accepted", func(t *testing.T) {
		dir := t.TempDir()
		caFile := filepath.Join(dir, "epp_ca.crt")
		require.NoError(t, os.WriteFile(caFile, []byte("dummy"), 0644))

		conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
		conf.EPPTLS = &EPPTLSConf{Insecure: false, CAFile: caFile}
		require.NoError(t, GslbBasicConfCheck(conf))
	})
}

func TestGslbBasicConfEPPJsonRoundTrip(t *testing.T) {
	data := []byte(`{
		"BalanceMode": "EPP",
		"EPPAddr": ["10.0.0.1:9002", "10.0.0.2:9002"],
		"EPPCheck": {
			"CheckInterval": "1s",
			"FailThreshold": 2,
			"Cooldown": "10s",
			"SuccessThreshold": 3
		},
		"EPPTimeout": {
			"Connect": "200ms",
			"Call": "1s"
		},
		"EPPTLS": {
			"Insecure": true
		}
	}`)

	var conf GslbBasicConf
	require.NoError(t, json.Unmarshal(data, &conf))
	require.NoError(t, GslbBasicConfCheck(&conf))

	assert.Equal(t, BalanceModeEPP, *conf.BalanceMode)
	assert.Equal(t, []string{"10.0.0.1:9002", "10.0.0.2:9002"}, *conf.EPPAddr)
	assert.Equal(t, "1s", *conf.EPPCheck.CheckInterval)
	assert.Equal(t, 2, *conf.EPPCheck.FailThreshold)
	assert.Equal(t, "10s", *conf.EPPCheck.Cooldown)
	assert.Equal(t, 3, *conf.EPPCheck.SuccessThreshold)
	assert.Equal(t, "200ms", *conf.EPPTimeout.Connect)
	assert.Equal(t, "1s", *conf.EPPTimeout.Call)
	assert.True(t, conf.EPPTLS.Insecure)
}

func TestGslbBasicConfNonEPPIgnoresEPPFields(t *testing.T) {
	conf := &GslbBasicConf{
		CrossRetry:  intPtr(0),
		RetryMax:    intPtr(2),
		HashConf:    &HashConf{},
		BalanceMode: strPtr(BalanceModeWrr),
		// invalid EPP fields are ignored in non-EPP mode
		EPPAddr:    strSlicePtr([]string{"bad-addr"}),
		EPPCheck:   &EPPCheckConf{CheckInterval: strPtr("bad")},
		EPPTimeout: &EPPTimeoutConf{Connect: strPtr("bad")},
		EPPTLS:     &EPPTLSConf{Insecure: false},
	}
	require.NoError(t, GslbBasicConfCheck(conf))
	// EPP fields are ignored (not defaulted) in non-EPP mode
	assert.Equal(t, "bad", *conf.EPPCheck.CheckInterval)
}

func TestGslbBasicConfCheckDisabled(t *testing.T) {
	conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
	conf.EPPCheck = &EPPCheckConf{Disabled: true}
	require.NoError(t, GslbBasicConfCheck(conf))
	assert.True(t, conf.EPPCheck.Disabled)
	assert.Equal(t, "2s", *conf.EPPCheck.CheckInterval)
}

func TestGslbBasicConfEPPBreakerDefaults(t *testing.T) {
	conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
	require.NoError(t, GslbBasicConfCheck(conf))
	require.NotNil(t, conf.EPPBreaker)
	assert.False(t, conf.EPPBreaker.Disabled)
	assert.Equal(t, 100, *conf.EPPBreaker.WindowSize)
	assert.Equal(t, 20, *conf.EPPBreaker.MinVolume)
	assert.Equal(t, 50, *conf.EPPBreaker.ErrorRatePercent)
	assert.Equal(t, "30s", *conf.EPPBreaker.OpenTimeout)
	assert.Equal(t, 30*time.Second, conf.EPPBreaker.OpenTimeoutDuration())
}

func TestGslbBasicConfEPPBreakerInvalid(t *testing.T) {
	base := eppGslbBasicConf([]string{"10.0.0.1:9002"})

	cases := []struct {
		name    string
		breaker *EPPBreakerConf
	}{
		{"window too small", &EPPBreakerConf{WindowSize: intPtr(0)}},
		{"min volume too small", &EPPBreakerConf{MinVolume: intPtr(0)}},
		{"min volume bigger than window", &EPPBreakerConf{WindowSize: intPtr(4), MinVolume: intPtr(5)}},
		{"error rate too small", &EPPBreakerConf{ErrorRatePercent: intPtr(0)}},
		{"error rate too big", &EPPBreakerConf{ErrorRatePercent: intPtr(101)}},
		{"bad open timeout", &EPPBreakerConf{OpenTimeout: strPtr("abc")}},
		{"non-positive open timeout", &EPPBreakerConf{OpenTimeout: strPtr("0s")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conf := *base
			conf.EPPBreaker = tc.breaker
			assert.Error(t, GslbBasicConfCheck(&conf))
		})
	}
}

func TestGslbBasicConfEPPBreakerValid(t *testing.T) {
	conf := eppGslbBasicConf([]string{"10.0.0.1:9002"})
	conf.EPPBreaker = &EPPBreakerConf{
		Disabled:         true,
		WindowSize:       intPtr(10),
		MinVolume:        intPtr(5),
		ErrorRatePercent: intPtr(80),
		OpenTimeout:      strPtr("10s"),
	}
	require.NoError(t, GslbBasicConfCheck(conf))
	assert.True(t, conf.EPPBreaker.Disabled)
}

func TestGslbBasicConfEPPBreakerJsonRoundTrip(t *testing.T) {
	data := []byte(`{
		"BalanceMode": "EPP",
		"EPPAddr": ["10.0.0.1:9002"],
		"EPPBreaker": {
			"WindowSize": 50,
			"MinVolume": 10,
			"ErrorRatePercent": 60,
			"OpenTimeout": "15s"
		}
	}`)

	var conf GslbBasicConf
	require.NoError(t, json.Unmarshal(data, &conf))
	require.NoError(t, GslbBasicConfCheck(&conf))
	assert.Equal(t, 50, *conf.EPPBreaker.WindowSize)
	assert.Equal(t, 10, *conf.EPPBreaker.MinVolume)
	assert.Equal(t, 60, *conf.EPPBreaker.ErrorRatePercent)
	assert.Equal(t, "15s", *conf.EPPBreaker.OpenTimeout)
}
