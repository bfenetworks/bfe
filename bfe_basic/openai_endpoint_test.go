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

package bfe_basic

import "testing"

func TestStripV1Prefix(t *testing.T) {
	cases := map[string]string{
		"/v1/chat/completions":                 "/chat/completions",
		"/v1/":                                 "/",
		"/v1":                                  "/v1",
		"/chat/completions":                    "/chat/completions",
		"/v10/xxx":                             "/v10/xxx",
		"/v1beta/x":                            "/v1beta/x",
		"/compatible-mode/v1/chat/completions": "/compatible-mode/v1/chat/completions",
		"":                                     "",
	}
	for path, want := range cases {
		if got := StripV1Prefix(path); got != want {
			t.Errorf("StripV1Prefix(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestIsOpenAIEndpoint(t *testing.T) {
	cases := map[string]bool{
		"/chat/completions":                    true,
		"/chat/completions/":                   true,
		"/completions":                         true,
		"/embeddings":                          true,
		"/models":                              true,
		"/models/gpt-4":                        true,
		"/responses":                           true,
		"/rerank":                              true,
		"/audio/speech":                        true,
		"/audio/translations":                  true,
		"/video/generations":                   true,
		"/images/generations":                  true,
		"/images/edits":                        true,
		"/moderations":                         true,
		"":                                     false,
		"/":                                    false,
		"/messages":                            false,
		"/v1/chat/completions":                 false, // must be stripped by the caller first
		"/compatible-mode/v1/chat/completions": false,
		"/modelsxyz":                           false,
		"/chat/completionsxyz":                 false,
		"/custom/path":                         false,
	}
	for path, want := range cases {
		if got := IsOpenAIEndpoint(path); got != want {
			t.Errorf("IsOpenAIEndpoint(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestDetectModeFromPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		// every mode, with and without the /v1 prefix, must agree
		{"/v1/chat/completions", ModeChat},
		{"/chat/completions", ModeChat},
		{"/v1/completions", ModeCompletion},
		{"/completions", ModeCompletion},
		{"/v1/embeddings", ModeEmbedding},
		{"/embeddings", ModeEmbedding},
		{"/v1/responses", ModeResponses},
		{"/responses", ModeResponses},
		{"/v1/rerank", ModeRerank},
		{"/rerank", ModeRerank},
		{"/v1/images/generations", ModeImageGeneration},
		{"/images/generations", ModeImageGeneration},
		{"/v1/images/edits", ModeImageEdit},
		{"/images/edits", ModeImageEdit},
		{"/v1/audio/speech", ModeAudioSpeech},
		{"/audio/speech", ModeAudioSpeech},
		{"/v1/audio/transcriptions", ModeAudioTranscription},
		{"/audio/transcriptions", ModeAudioTranscription},
		{"/v1/video/generations", ModeVideoGeneration},
		{"/video/generations", ModeVideoGeneration},
		// endpoints without a dedicated mode keep the default
		{"/v1/models", ModeChat},
		{"/models", ModeChat},
		{"/models/gpt-4", ModeChat},
		{"/v1/models/gpt-4", ModeChat},
		// boundaries: unchanged from the legacy /v1-only behavior
		{"/v10/xxx", ModeChat},
		{"/v1beta/models/gemini:generateContent", ModeChat},
		{"/v1/messages", ModeChat}, // anthropic entry stays default
		{"/messages", ModeChat},
		{"/compatible-mode/v1/chat/completions", ModeChat}, // provider-native path
		{"/v1", ModeChat},
		{"/v1/", ModeChat},
		{"/", ModeChat},
		{"", ModeChat},
		{"/custom/path", ModeChat},
	}
	for _, c := range cases {
		if got := DetectModeFromPath(c.path); got != c.want {
			t.Errorf("DetectModeFromPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
