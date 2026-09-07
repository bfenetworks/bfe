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
		if entry.GetPriceInt("", PriceInputCostPerTokenInt) != 100 {
			t.Errorf("input cost int = %v, want 100", entry.GetPriceInt("", PriceInputCostPerTokenInt))
		}
		if entry.GetPriceInt("", PriceOutputCostPerTokenInt) != 200 {
			t.Errorf("output cost int = %v, want 200", entry.GetPriceInt("", PriceOutputCostPerTokenInt))
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
		if entry.GetPriceInt("", PriceCacheReadInputTokenCostInt) != 45 {
			t.Errorf("cache read cost int = %v, want 45", entry.GetPriceInt("", PriceCacheReadInputTokenCostInt))
		}
		if entry.GetPriceInt("", PriceCacheCreationInputTokenCostInt) != 565 {
			t.Errorf("cache write cost int = %v, want 565", entry.GetPriceInt("", PriceCacheCreationInputTokenCostInt))
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
		if entry.GetPriceInt("", PriceInputCostPerAudioTokenInt) != 2288 {
			t.Errorf("audio input cost int = %v, want 2288", entry.GetPriceInt("", PriceInputCostPerAudioTokenInt))
		}
		if entry.GetPriceInt("", PriceOutputCostPerAudioTokenInt) != 4576 {
			t.Errorf("audio output cost int = %v, want 4576", entry.GetPriceInt("", PriceOutputCostPerAudioTokenInt))
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

func TestGetPriceInt(t *testing.T) {
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

	if got := entry.GetPriceInt("", PriceInputCostPerTokenInt); got != 100000000 {
		t.Errorf("default input cost = %d, want 100000000", got)
	}
	if got := entry.GetPriceInt("peak", PriceInputCostPerTokenInt); got != 1000000000 {
		t.Errorf("peak input cost = %d, want 1000000000", got)
	}
	if got := entry.GetPriceInt("peak", PriceOutputCostPerTokenInt); got != 200000000 {
		t.Errorf("peak output cost (fallback) = %d, want 200000000", got)
	}
	if got := entry.GetPriceInt("nonexistent", PriceInputCostPerTokenInt); got != 100000000 {
		t.Errorf("nonexistent tier fallback = %d, want 100000000", got)
	}
}
