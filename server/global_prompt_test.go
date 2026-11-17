package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGlobalPromptValidation(t *testing.T) {
	tests := []struct {
		name string
		cfg  globalPromptSettings
		ok   bool
	}{
		{name: "default", ok: true},
		{name: "disabled draft", cfg: globalPromptSettings{Prompt: "draft"}, ok: true},
		{name: "enabled empty", cfg: globalPromptSettings{Enabled: true}},
		{name: "enabled whitespace", cfg: globalPromptSettings{Enabled: true, Prompt: " \n\t"}},
		{name: "literal template", cfg: globalPromptSettings{Enabled: true, Prompt: "{{.Unknown}}"}, ok: true},
		{name: "at limit", cfg: globalPromptSettings{Prompt: strings.Repeat("x", maxGlobalPromptBytes)}, ok: true},
		{name: "over limit", cfg: globalPromptSettings{Prompt: strings.Repeat("x", maxGlobalPromptBytes+1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.validate(); (err == nil) != tt.ok {
				t.Fatalf("validation result: %v", err)
			}
		})
	}
}

func TestDecodeGlobalPromptSettings(t *testing.T) {
	invalid := []string{
		`{`,
		`{"unknown":true}`,
		`{"enabled":"yes"}`,
		`{} {}`,
		`[]`,
		`{"enabled":true,"prompt":""}`,
	}
	for _, body := range invalid {
		r := httptest.NewRequest(http.MethodPut, "/api/global-prompt", strings.NewReader(body))
		if _, err := decodeGlobalPromptSettings(httptest.NewRecorder(), r); err == nil {
			t.Fatalf("invalid configuration accepted: %s", body)
		}
	}

	const body = `{"enabled":true,"prompt":"  示例 {{.Unknown}}\n"}`
	r := httptest.NewRequest(http.MethodPut, "/api/global-prompt", strings.NewReader(body))
	cfg, err := decodeGlobalPromptSettings(httptest.NewRecorder(), r)
	if err != nil || cfg.Prompt != "  示例 {{.Unknown}}\n" {
		t.Fatalf("plain text was modified: %+v %v", cfg, err)
	}
}
