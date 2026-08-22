// Copyright (c) 2019 The BFE Authors.
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
	"testing"
)

func TestClusterConfLoad_1(t *testing.T) {
	config, err := ClusterConfLoad("./testdata/cluster_conf_1.conf")
	if err != nil {
		t.Errorf("get err from ClusterConfLoad():%s", err.Error())
		return
	}

	if len(*config.Config) != 2 {
		t.Error("len(config.Config) should be 2")
		return
	}
}

func TestClusterConfLoad_2(t *testing.T) {
	if _, err := ClusterConfLoad("./testdata/cluster_conf_2.conf"); err == nil {
		t.Error("it should be error in ClusterConfLoad()")
		return
	}
}

func TestClusterConfLoad_3(t *testing.T) {
	config, err := ClusterConfLoad("./testdata/cluster_conf_3.conf")
	if err != nil {
		t.Errorf("ClusterConfLoad() error: %v", err)
		return
	}
	schem := *(*config.Config)["p2"].CheckConf.Schem
	if schem != "tcp" {
		t.Errorf("schem should be tcp, not %s", schem)
	}
}

func TestClusterConfLoad_4(t *testing.T) {
	_, err := ClusterConfLoad("./testdata/cluster_conf_4.conf")
	if err == nil {
		t.Error("it should be error in ClusterConfLoad()")
		return
	}
}

func TestClusterConfLoad_6(t *testing.T) {
	_, err := ClusterConfLoad("./testdata/cluster_conf_6.conf")
	if err == nil {
		t.Error("it should be error in ClusterConfLoad()")
		return
	}
}

func TestStatusCodeRange(t *testing.T) {
	var (
		statusCode       = "400"
		statusCodeRanges = map[string]bool{
			"200":         false,
			"2xx":         false,
			"4x0":         true,
			"43x":         false,
			"400":         true,
			"40x":         true,
			"4xx":         true,
			"x00":         true,
			"x0x":         true,
			"404|30x|2xx": false,
			"200|40x|3xx": true,
			"2xx|4xx|3xx": true,
		}
		worngRange = []string{
			"4000",
			"x4x&5xx|6xx|111",
			"[200-300]",
			"|400",
		}
	)
	t.Run("checkStatusCodeRange", func(t *testing.T) {
		var err error
		for statusCodeRange, _ := range statusCodeRanges {
			if err = checkStatusCodeRange(&statusCodeRange); err != nil {
				t.Error(err)
			}
		}
		for _, w := range worngRange {
			if err = checkStatusCodeRange(&w); err == nil {
				t.Errorf("assertOk=false, ok=true, statusCodeRange=%s", w)
			} else {
				t.Log(err)
			}
		}
	})
	t.Run("MatchStatusCodeRange", func(t *testing.T) {
		for statusCodeRange, assert := range statusCodeRanges {
			ok, err := MatchStatusCodeRange(statusCode, statusCodeRange)
			if ok != assert {
				t.Errorf("statusCode=%s, statusCodeRange=%s, assertOk=%v, ok=%v", statusCode, statusCodeRange, assert, ok)
			}
			if err != nil {
				t.Logf("assertOk=%v, ok=%v, err_msg=%s", assert, ok, err.Error())
			}
		}
	})
}


func TestModelTableCheck(t *testing.T) {
	t.Run("valid RMB table", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{
					Model: "deepseek-chat",
					Mode:  "chat",
					Prices: map[string]float64{
						PriceInputCostPerToken:  0.000001,
						PriceOutputCostPerToken: 0.000002,
					},
				},
			},
		}
		if err := ModelTableCheck(table); err != nil {
			t.Fatalf("ModelTableCheck failed: %v", err)
		}
		if table.priceIndex == nil {
			t.Fatal("priceIndex should be built")
		}
		entry := LookupModelPrice(table, "deepseek-chat", "chat")
		if entry == nil {
			t.Fatal("LookupModelPrice should return entry")
		}
		if entry.Prices[PriceInputCostPerTokenInt] != 100 {
			t.Errorf("input cost int = %v, want 100", entry.Prices[PriceInputCostPerTokenInt])
		}
		if entry.Prices[PriceOutputCostPerTokenInt] != 200 {
			t.Errorf("output cost int = %v, want 200", entry.Prices[PriceOutputCostPerTokenInt])
		}
	})

	t.Run("valid RMB table with cache prices", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{
					Model: "claude-opus-4-6",
					Mode:  "chat",
					Prices: map[string]float64{
						PriceInputCostPerToken:           0.000004525,
						PriceOutputCostPerToken:          0.000022625,
						PriceCacheReadInputTokenCost:     0.0000004525,
						PriceCacheCreationInputTokenCost: 0.00000565625,
					},
				},
			},
		}
		if err := ModelTableCheck(table); err != nil {
			t.Fatalf("ModelTableCheck failed: %v", err)
		}
		entry := LookupModelPrice(table, "claude-opus-4-6", "chat")
		if entry == nil {
			t.Fatal("LookupModelPrice should return entry")
		}
		if entry.Prices[PriceCacheReadInputTokenCostInt] != 45 {
			t.Errorf("cache read cost int = %v, want 45", entry.Prices[PriceCacheReadInputTokenCostInt])
		}
		if entry.Prices[PriceCacheCreationInputTokenCostInt] != 565 {
			t.Errorf("cache write cost int = %v, want 565", entry.Prices[PriceCacheCreationInputTokenCostInt])
		}
	})

	t.Run("invalid currency", func(t *testing.T) {
		table := &ModelTable{
			Currency: "USD",
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: map[string]float64{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for invalid currency")
		}
	})

	t.Run("negative price", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: map[string]float64{PriceInputCostPerToken: -1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for negative price")
		}
	})

	t.Run("duplicate model mode", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: map[string]float64{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
				{Model: "m", Mode: "chat", Prices: map[string]float64{PriceInputCostPerToken: 2, PriceOutputCostPerToken: 2}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for duplicate model/mode")
		}
	})

	t.Run("missing model or mode", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{Model: "", Mode: "chat", Prices: map[string]float64{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for empty model")
		}
	})
}

func TestAIConfCheck(t *testing.T) {
	t.Run("strip prefix without match prefix", func(t *testing.T) {
		conf := &AIConf{StripPrefix: true}
		if err := AIConfCheck(conf); err == nil {
			t.Error("expected error when StripPrefix=true but MatchPrefix is empty")
		}
	})

	t.Run("match prefix without trailing slash", func(t *testing.T) {
		conf := &AIConf{StripPrefix: true, MatchPrefix: "openrouter"}
		if err := AIConfCheck(conf); err == nil {
			t.Error("expected error when MatchPrefix does not end with '/'")
		}
	})

	t.Run("valid strip prefix config", func(t *testing.T) {
		conf := &AIConf{StripPrefix: true, MatchPrefix: "openrouter/"}
		if err := AIConfCheck(conf); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("strip prefix disabled", func(t *testing.T) {
		conf := &AIConf{StripPrefix: false, MatchPrefix: ""}
		if err := AIConfCheck(conf); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
