package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const globalPromptSettingsKey = "agent.global_prompt"
const maxGlobalPromptBytes = 128 << 10

type globalPromptSettings struct {
	Enabled bool   `json:"enabled"`
	Prompt  string `json:"prompt"`
}

func (cfg globalPromptSettings) validate() error {
	if len(cfg.Prompt) > maxGlobalPromptBytes {
		return fmt.Errorf("prompt exceeds %d UTF-8 bytes", maxGlobalPromptBytes)
	}
	if cfg.Enabled && strings.TrimSpace(cfg.Prompt) == "" {
		return fmt.Errorf("prompt is empty")
	}
	return nil
}

func (s *Server) loadGlobalPromptSettings() (globalPromptSettings, error) {
	var cfg globalPromptSettings
	if s.m.pg == nil {
		return cfg, fmt.Errorf("settings store is unavailable")
	}
	raw, ok, err := s.m.pg.GetSetting(globalPromptSettingsKey)
	if err != nil {
		return cfg, fmt.Errorf("load global prompt: %w", err)
	}
	if ok {
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return cfg, fmt.Errorf("decode global prompt: %w", err)
		}
	}
	return cfg, cfg.validate()
}

func (s *Server) effectiveGlobalSystemPrompt() string {
	cfg, err := s.loadGlobalPromptSettings()
	if err != nil {
		log.Printf("[global-prompt] load failed: %v", err)
		return ""
	}
	if !cfg.Enabled {
		return ""
	}
	return cfg.Prompt
}

func (s *Server) getGlobalPromptSettings(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.loadGlobalPromptSettings()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "读取全局提示词失败")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func decodeGlobalPromptSettings(w http.ResponseWriter, r *http.Request) (globalPromptSettings, error) {
	var cfg globalPromptSettings
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode global prompt request: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return cfg, fmt.Errorf("expected a single JSON object")
	}
	return cfg, cfg.validate()
}

func (s *Server) putGlobalPromptSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := decodeGlobalPromptSettings(w, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "配置格式错误；启用时正文须非空，长度上限为 128 KiB")
		return
	}
	if s.m.pg == nil {
		writeErr(w, http.StatusServiceUnavailable, "设置存储暂时不可用")
		return
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "编码全局提示词失败")
		return
	}
	if err := s.m.pg.SetSettingContext(r.Context(), globalPromptSettingsKey, string(raw)); err != nil {
		writeErr(w, http.StatusInternalServerError, "保存全局提示词失败")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}
