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
	"time"
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
					Prices: PriceMap{
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
		if entry.GetPrice("", PriceInputCostPerToken) != 0.000001 {
			t.Errorf("input cost = %v, want 0.000001", entry.GetPrice("", PriceInputCostPerToken))
		}
		if entry.GetPrice("", PriceOutputCostPerToken) != 0.000002 {
			t.Errorf("output cost = %v, want 0.000002", entry.GetPrice("", PriceOutputCostPerToken))
		}
	})

	t.Run("valid RMB table with cache prices", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{
					Model: "claude-opus-4-6",
					Mode:  "chat",
					Prices: PriceMap{
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
		if entry.GetPrice("", PriceCacheReadInputTokenCost) != 0.0000004525 {
			t.Errorf("cache read cost = %v, want 0.0000004525", entry.GetPrice("", PriceCacheReadInputTokenCost))
		}
		if entry.GetPrice("", PriceCacheCreationInputTokenCost) != 0.00000565625 {
			t.Errorf("cache write cost = %v, want 0.00000565625", entry.GetPrice("", PriceCacheCreationInputTokenCost))
		}
	})

	t.Run("valid RMB table with audio prices", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{
					Model: "gpt-audio-1.5",
					Mode:  "chat",
					Prices: PriceMap{
						PriceInputCostPerToken:       0.00000178,
						PriceOutputCostPerToken:      0.00000715,
						PriceInputCostPerAudioToken:  0.00002288,
						PriceOutputCostPerAudioToken: 0.00004576,
					},
				},
			},
		}
		if err := ModelTableCheck(table); err != nil {
			t.Fatalf("ModelTableCheck failed: %v", err)
		}
		entry := LookupModelPrice(table, "gpt-audio-1.5", "chat")
		if entry == nil {
			t.Fatal("LookupModelPrice should return entry")
		}
		if entry.GetPrice("", PriceInputCostPerAudioToken) != 0.00002288 {
			t.Errorf("audio input cost = %v, want 0.00002288", entry.GetPrice("", PriceInputCostPerAudioToken))
		}
		if entry.GetPrice("", PriceOutputCostPerAudioToken) != 0.00004576 {
			t.Errorf("audio output cost = %v, want 0.00004576", entry.GetPrice("", PriceOutputCostPerAudioToken))
		}
	})

	t.Run("invalid currency", func(t *testing.T) {
		table := &ModelTable{
			Currency: "USD",
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
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
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: -1, PriceOutputCostPerToken: 1}},
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
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 2, PriceOutputCostPerToken: 2}},
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
				{Model: "", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
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

	t.Run("key policy affinity defaults", func(t *testing.T) {
		conf := &AIConf{
			KeyPolicy: &AIKeyPolicy{
				Strategy:        "weighted_random",
				MaxRetries:      3,
				SessionAffinity: true,
			},
		}
		if err := AIConfCheck(conf); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if conf.KeyPolicy.SessionAffinityTTL != 600 {
			t.Errorf("expected SessionAffinityTTL default 600, got %d", conf.KeyPolicy.SessionAffinityTTL)
		}
		if conf.KeyPolicy.SessionAffinityRedisPrefix != "bfe:ai:key_affinity" {
			t.Errorf("expected default redis prefix, got %s", conf.KeyPolicy.SessionAffinityRedisPrefix)
		}
	})

	t.Run("key policy negative ttl error", func(t *testing.T) {
		conf := &AIConf{
			KeyPolicy: &AIKeyPolicy{
				SessionAffinityTTL: -1,
			},
		}
		if err := AIConfCheck(conf); err == nil {
			t.Error("expected error when SessionAffinityTTL < 0")
		}
	})
}

func TestModelTableCheck_Tiers(t *testing.T) {
	validPeakTier := PriceTier{
		Name: "peak",
		TimeRanges: []TimeRange{
			{Weekdays: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "12:00"},
			{Weekdays: []int{1, 2, 3, 4, 5}, Start: "14:00", End: "18:00"},
		},
	}

	t.Run("valid peak tier", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			TimeZone: "Asia/Shanghai",
			Tiers:    []PriceTier{validPeakTier},
			Models: []ModelPrice{
				{
					Model: "m",
					Mode:  "chat",
					Prices: PriceMap{
						PriceInputCostPerToken:  1,
						PriceOutputCostPerToken: 1,
					},
				},
			},
		}
		if err := ModelTableCheck(table); err != nil {
			t.Fatalf("ModelTableCheck failed: %v", err)
		}
		if table.tz == nil {
			t.Error("tz should be set")
		}
		if table.tierIndex["peak"] == nil {
			t.Error("tierIndex should contain peak")
		}
	})

	t.Run("default timezone", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Tiers:    []PriceTier{validPeakTier},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err != nil {
			t.Fatalf("ModelTableCheck failed: %v", err)
		}
		if table.TimeZone != "Asia/Shanghai" {
			t.Errorf("timezone = %s, want Asia/Shanghai", table.TimeZone)
		}
	})

	t.Run("invalid timezone", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			TimeZone: "Mars/Phobos",
			Tiers:    []PriceTier{validPeakTier},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for invalid timezone")
		}
	})

	t.Run("unsupported tier name", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Tiers: []PriceTier{
				{Name: "off_peak", TimeRanges: []TimeRange{{Start: "00:00", End: "09:00"}}},
			},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for unsupported tier name")
		}
	})

	t.Run("empty tier name", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Tiers:    []PriceTier{{Name: "", TimeRanges: []TimeRange{{Start: "00:00", End: "09:00"}}}},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for empty tier name")
		}
	})

	t.Run("no time ranges", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Tiers:    []PriceTier{{Name: "peak"}},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for tier without time ranges")
		}
	})

	t.Run("invalid weekday", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Tiers: []PriceTier{
				{Name: "peak", TimeRanges: []TimeRange{{Weekdays: []int{7}, Start: "09:00", End: "12:00"}}},
			},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for invalid weekday")
		}
	})

	t.Run("end before start", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Tiers: []PriceTier{
				{Name: "peak", TimeRanges: []TimeRange{{Weekdays: []int{1}, Start: "12:00", End: "09:00"}}},
			},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error when end <= start")
		}
	})

	t.Run("overlapping time ranges", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Tiers: []PriceTier{
				{
					Name: "peak",
					TimeRanges: []TimeRange{
						{Weekdays: []int{1}, Start: "09:00", End: "12:00"},
						{Weekdays: []int{1}, Start: "11:00", End: "14:00"},
					},
				},
			},
			Models: []ModelPrice{
				{Model: "m", Mode: "chat", Prices: PriceMap{PriceInputCostPerToken: 1, PriceOutputCostPerToken: 1}},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for overlapping time ranges")
		}
	})

	t.Run("unsupported tier price name", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{
					Model: "m",
					Mode:  "chat",
					Prices: PriceMap{
						PriceInputCostPerToken:  1,
						PriceOutputCostPerToken: 1,
					},
					TierPrices: TierPriceMap{
						"off_peak": {PriceInputCostPerToken: 2},
					},
				},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for unsupported tier price name")
		}
	})

	t.Run("negative tier price", func(t *testing.T) {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{
					Model: "m",
					Mode:  "chat",
					Prices: PriceMap{
						PriceInputCostPerToken:  1,
						PriceOutputCostPerToken: 1,
					},
					TierPrices: TierPriceMap{
						"peak": {PriceInputCostPerToken: -1},
					},
				},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Error("expected error for negative tier price")
		}
	})
}

func TestActiveTierName(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location failed: %v", err)
	}

	table := &ModelTable{
		Currency: "RMB",
		TimeZone: "Asia/Shanghai",
		Tiers: []PriceTier{
			{
				Name: "peak",
				TimeRanges: []TimeRange{
					{Weekdays: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "12:00"},
					{Weekdays: []int{1, 2, 3, 4, 5}, Start: "14:00", End: "18:00"},
				},
			},
		},
	}
	if err := ModelTableCheck(table); err != nil {
		t.Fatalf("ModelTableCheck failed: %v", err)
	}

	cases := []struct {
		name     string
		moment   time.Time
		expected string
	}{
		{"monday 10:00 peak", time.Date(2026, 8, 24, 10, 0, 0, 0, shanghai), "peak"},
		{"monday 13:00 off-peak", time.Date(2026, 8, 24, 13, 0, 0, 0, shanghai), ""},
		{"saturday 10:00 weekend", time.Date(2026, 8, 22, 10, 0, 0, 0, shanghai), ""},
		{"monday 18:00 half-open end", time.Date(2026, 8, 24, 18, 0, 0, 0, shanghai), ""},
		{"monday 09:00 half-open start", time.Date(2026, 8, 24, 9, 0, 0, 0, shanghai), "peak"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := table.ActiveTierName(c.moment); got != c.expected {
				t.Errorf("ActiveTierName = %q, want %q", got, c.expected)
			}
		})
	}
}

func TestGetPrice(t *testing.T) {
	table := &ModelTable{
		Currency: "RMB",
		Models: []ModelPrice{
			{
				Model: "m",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerToken:  1,
					PriceOutputCostPerToken: 2,
				},
				TierPrices: TierPriceMap{
					"peak": {
						PriceInputCostPerToken: 10,
					},
				},
			},
		},
	}
	if err := ModelTableCheck(table); err != nil {
		t.Fatalf("ModelTableCheck failed: %v", err)
	}
	entry := LookupModelPrice(table, "m", "chat")
	if entry == nil {
		t.Fatal("LookupModelPrice should return entry")
	}

	if got := entry.GetPrice("", PriceInputCostPerToken); got != 1 {
		t.Errorf("default input cost = %v, want 1", got)
	}
	if got := entry.GetPrice("peak", PriceInputCostPerToken); got != 10 {
		t.Errorf("peak input cost = %v, want 10", got)
	}
	if got := entry.GetPrice("peak", PriceOutputCostPerToken); got != 2 {
		t.Errorf("peak output cost (fallback) = %v, want 2", got)
	}
	if got := entry.GetPrice("nonexistent", PriceInputCostPerToken); got != 1 {
		t.Errorf("nonexistent tier fallback = %v, want 1", got)
	}
}

func TestGetPriceHighPrecision(t *testing.T) {
	// Prices with more than 8 decimal places must survive config loading
	// without being truncated to fixed-point integers.
	table := &ModelTable{
		Currency: "RMB",
		Models: []ModelPrice{
			{
				Model: "qwen2.5-omni-7b",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerToken:  6.0168984e-09,
					PriceOutputCostPerToken: 7.6234102728e-08,
				},
				TierPrices: TierPriceMap{
					"peak": {
						PriceOutputCostPerToken: 1.52468205456e-07,
					},
				},
			},
		},
	}
	if err := ModelTableCheck(table); err != nil {
		t.Fatalf("ModelTableCheck failed: %v", err)
	}
	entry := LookupModelPrice(table, "qwen2.5-omni-7b", "chat")
	if entry == nil {
		t.Fatal("LookupModelPrice should return entry")
	}

	if got := entry.GetPrice("", PriceOutputCostPerToken); got != 7.6234102728e-08 {
		t.Errorf("high precision output cost = %v, want 7.6234102728e-08", got)
	}
	if got := entry.GetPrice("peak", PriceOutputCostPerToken); got != 1.52468205456e-07 {
		t.Errorf("high precision peak output cost = %v, want 1.52468205456e-07", got)
	}
	// input price is not configured in the peak tier: fall back to default.
	if got := entry.GetPrice("peak", PriceInputCostPerToken); got != 6.0168984e-09 {
		t.Errorf("peak input cost (fallback) = %v, want 6.0168984e-09", got)
	}
}

func TestModelTableCheck_CacheWrite1hAndLengthTierKeys(t *testing.T) {
	// New price keys load fine when non-negative.
	table := &ModelTable{
		Currency: "RMB",
		Models: []ModelPrice{
			{
				Model: "gpt-5.5",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerToken:                 2.431e-05,
					PriceOutputCostPerToken:                0.00014586,
					PriceCacheCreationInputTokenCost1h:     6.154e-05,
					PriceInputCostPerTokenAbove272kTokens:  4.862e-05,
					PriceOutputCostPerTokenAbove272kTokens: 0.00021879,
				},
			},
			{
				Model: "claude-opus-4-8",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerToken:             3.077e-05,
					PriceCacheCreationInputTokenCost:   3.84625e-05,
					PriceCacheCreationInputTokenCost1h: 6.154e-05,
				},
			},
		},
	}
	if err := ModelTableCheck(table); err != nil {
		t.Fatalf("ModelTableCheck failed: %v", err)
	}

	gpt := LookupModelPrice(table, "gpt-5.5", "chat")
	if len(gpt.lengthTiers) != 1 {
		t.Fatalf("gpt-5.5 lengthTiers = %d, want 1", len(gpt.lengthTiers))
	}
	if gpt.lengthTiers[0].threshold != 272000 {
		t.Errorf("tier threshold = %d, want 272000", gpt.lengthTiers[0].threshold)
	}

	claude := LookupModelPrice(table, "claude-opus-4-8", "chat")
	if len(claude.lengthTiers) != 0 {
		t.Errorf("claude lengthTiers = %d, want 0", len(claude.lengthTiers))
	}
	if got := claude.GetPrice("", PriceCacheCreationInputTokenCost1h); got != 6.154e-05 {
		t.Errorf("cache write 1h price = %v, want 6.154e-05", got)
	}
}

func TestModelTableCheck_NegativeNewKeys(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		value float64
	}{
		{"negative 1h price", PriceCacheCreationInputTokenCost1h, -1},
		{"negative tier input price", PriceInputCostPerTokenAbove272kTokens, -0.1},
		{"negative tier output price", PriceOutputCostPerTokenAbove512kTokens, -0.1},
	}
	for _, c := range cases {
		table := &ModelTable{
			Currency: "RMB",
			Models: []ModelPrice{
				{
					Model:  "m",
					Mode:   "chat",
					Prices: PriceMap{c.key: c.value},
				},
			},
		}
		if err := ModelTableCheck(table); err == nil {
			t.Errorf("%s: ModelTableCheck should reject negative price", c.name)
		}
	}
}

func TestModelTableCheck_LengthTiersSorted(t *testing.T) {
	// All four tiers configured; lengthTiers must be ordered by ascending
	// threshold regardless of map iteration order.
	table := &ModelTable{
		Currency: "RMB",
		Models: []ModelPrice{
			{
				Model: "MiniMax-M3",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerTokenAbove512kTokens:  1,
					PriceOutputCostPerTokenAbove512kTokens: 2,
					PriceInputCostPerTokenAbove272kTokens:  3,
					PriceOutputCostPerTokenAbove272kTokens: 4,
					PriceInputCostPerTokenAbove256kTokens:  5,
					PriceOutputCostPerTokenAbove256kTokens: 6,
					PriceInputCostPerTokenAbove200kTokens:  7,
					PriceOutputCostPerTokenAbove200kTokens: 8,
				},
			},
		},
	}
	if err := ModelTableCheck(table); err != nil {
		t.Fatalf("ModelTableCheck failed: %v", err)
	}
	entry := LookupModelPrice(table, "MiniMax-M3", "chat")
	want := []int64{200000, 256000, 272000, 512000}
	if len(entry.lengthTiers) != len(want) {
		t.Fatalf("lengthTiers = %d, want %d", len(entry.lengthTiers), len(want))
	}
	for i, w := range want {
		if entry.lengthTiers[i].threshold != w {
			t.Errorf("lengthTiers[%d].threshold = %d, want %d", i, entry.lengthTiers[i].threshold, w)
		}
	}
}

func TestGetLengthTierPrice(t *testing.T) {
	table := &ModelTable{
		Currency: "RMB",
		Models: []ModelPrice{
			{
				Model: "gpt-5.5",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerToken:                 2.431e-05,
					PriceOutputCostPerToken:                0.00014586,
					PriceInputCostPerTokenAbove272kTokens:  4.862e-05,
					PriceOutputCostPerTokenAbove272kTokens: 0.00021879,
				},
				TierPrices: TierPriceMap{
					"peak": {
						PriceInputCostPerTokenAbove272kTokens: 9.724e-05,
					},
				},
			},
			{
				// only the input tier key is configured
				Model: "qwen3.6-flash",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerToken:                1e-05,
					PriceOutputCostPerToken:               2e-05,
					PriceInputCostPerTokenAbove256kTokens: 3e-05,
				},
			},
			{
				Model: "no-tier-model",
				Mode:  "chat",
				Prices: PriceMap{
					PriceInputCostPerToken:  1e-05,
					PriceOutputCostPerToken: 2e-05,
				},
			},
		},
	}
	if err := ModelTableCheck(table); err != nil {
		t.Fatalf("ModelTableCheck failed: %v", err)
	}

	gpt := LookupModelPrice(table, "gpt-5.5", "chat")

	// below the threshold: not ok, caller uses base prices
	if _, _, ok := gpt.GetLengthTierPrice("", 272000); ok {
		t.Error("promptTokens == threshold should not select the tier")
	}
	if _, _, ok := gpt.GetLengthTierPrice("", 100); ok {
		t.Error("small promptTokens should not select the tier")
	}

	// above the threshold: tier prices from default Prices
	in, out, ok := gpt.GetLengthTierPrice("", 300000)
	if !ok {
		t.Fatal("promptTokens 300000 should select the 272k tier")
	}
	if in != 4.862e-05 || out != 0.00021879 {
		t.Errorf("tier prices = (%v, %v), want (4.862e-05, 0.00021879)", in, out)
	}

	// tier priority: TierPrices.peak overrides the input side only
	in, out, ok = gpt.GetLengthTierPrice("peak", 300000)
	if !ok {
		t.Fatal("peak tier lookup should select the tier")
	}
	if in != 9.724e-05 {
		t.Errorf("peak tier input price = %v, want 9.724e-05", in)
	}
	if out != 0.00021879 {
		t.Errorf("peak tier output price (fallback to default) = %v, want 0.00021879", out)
	}

	// input-only tier key: output side returns -1 (caller keeps base price)
	qwen := LookupModelPrice(table, "qwen3.6-flash", "chat")
	in, out, ok = qwen.GetLengthTierPrice("", 300000)
	if !ok {
		t.Fatal("input-only tier key should still select the tier")
	}
	if in != 3e-05 {
		t.Errorf("tier input price = %v, want 3e-05", in)
	}
	if out != -1 {
		t.Errorf("tier output price = %v, want -1 (not configured)", out)
	}

	// no tier keys at all: ok=false
	noTier := LookupModelPrice(table, "no-tier-model", "chat")
	if _, _, ok := noTier.GetLengthTierPrice("", 1000000); ok {
		t.Error("model without tier keys should return ok=false")
	}
}
