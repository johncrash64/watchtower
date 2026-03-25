package config

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"

	"gopkg.in/yaml.v3"
)

const DefaultConfigFile = "watchtower.yaml"

type Config struct {
	StudiesDir string    `yaml:"studies_dir"`
	Web        WebConfig `yaml:"web"`
	LLM        LLMConfig `yaml:"llm"`
}

type WebConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type LLMConfig struct {
	DefaultMode string        `yaml:"default_mode"`
	OpenAI      OpenAIConfig  `yaml:"openai"`
	Gemini      GeminiConfig  `yaml:"gemini"`
	Local       LocalAIConfig `yaml:"local"`
	GLM         GLMConfig     `yaml:"glm"`
}

type OpenAIConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
	URL    string `yaml:"url"`
}

type GeminiConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
	URL    string `yaml:"url"`
}

type LocalAIConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
	Model   string `yaml:"model"`
	APIKey  string `yaml:"api_key"`
}

type GLMConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
	URL    string `yaml:"url"`
}

func Defaults() Config {
	return Config{
		StudiesDir: "studies",
		Web: WebConfig{
			Host: "127.0.0.1",
			Port: 8088,
		},
		LLM: LLMConfig{
			DefaultMode: "balanced",
			OpenAI: OpenAIConfig{
				Model: "gpt-4.1-mini",
				URL:   "https://api.openai.com/v1/chat/completions",
			},
			Gemini: GeminiConfig{
				Model: "gemini-2.0-flash",
				URL:   "https://generativelanguage.googleapis.com/v1beta",
			},
			Local: LocalAIConfig{
				Enabled: true,
				URL:     "http://127.0.0.1:11434/v1/chat/completions",
				Model:   "llama3.1:8b",
			},
			GLM: GLMConfig{
				Model: "glm-4-flash",
				URL:   "https://open.bigmodel.cn/api/paas/v4/chat/completions",
			},
		},
	}
}

func Load(configPath string) (Config, error) {
	cfg := Defaults()

	if configPath == "" {
		configPath = DefaultConfigFile
	}

	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}

	overrideFromEnv(&cfg)

	absStudies, err := filepath.Abs(cfg.StudiesDir)
	if err == nil {
		cfg.StudiesDir = absStudies
	}

	return cfg, nil
}

func overrideFromEnv(cfg *Config) {
	if v, ok := os.LookupEnv("WATCHTOWER_STUDIES_DIR"); ok && v != "" {
		cfg.StudiesDir = v
	}
	if v, ok := os.LookupEnv("WATCHTOWER_WEB_HOST"); ok && v != "" {
		cfg.Web.Host = v
	}
	if v, ok := os.LookupEnv("WATCHTOWER_WEB_PORT"); ok && v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Web.Port = parsed
		}
	}
	if v, ok := os.LookupEnv("WATCHTOWER_ANALYSIS_MODE"); ok && v != "" {
		cfg.LLM.DefaultMode = v
	}

	if v, ok := os.LookupEnv("OPENAI_API_KEY"); ok {
		cfg.LLM.OpenAI.APIKey = v
	}
	if v, ok := os.LookupEnv("OPENAI_BASE_URL"); ok && v != "" {
		cfg.LLM.OpenAI.URL = v
	}
	if v, ok := os.LookupEnv("OPENAI_MODEL"); ok && v != "" {
		cfg.LLM.OpenAI.Model = v
	}

	if v, ok := os.LookupEnv("GEMINI_API_KEY"); ok {
		cfg.LLM.Gemini.APIKey = v
	}
	if v, ok := os.LookupEnv("GEMINI_MODEL"); ok && v != "" {
		cfg.LLM.Gemini.Model = v
	}
	if v, ok := os.LookupEnv("GEMINI_BASE_URL"); ok && v != "" {
		cfg.LLM.Gemini.URL = v
	}

	if v, ok := os.LookupEnv("LOCAL_LLM_URL"); ok && v != "" {
		cfg.LLM.Local.URL = v
	}
	if v, ok := os.LookupEnv("LOCAL_LLM_MODEL"); ok && v != "" {
		cfg.LLM.Local.Model = v
	}
	if v, ok := os.LookupEnv("LOCAL_LLM_API_KEY"); ok {
		cfg.LLM.Local.APIKey = v
	}
	if v, ok := os.LookupEnv("LOCAL_LLM_ENABLED"); ok && v != "" {
		cfg.LLM.Local.Enabled = v == "1" || v == "true" || v == "TRUE"
	}

	if v, ok := os.LookupEnv("GLM_API_KEY"); ok {
		cfg.LLM.GLM.APIKey = v
	}
	if v, ok := os.LookupEnv("GLM_MODEL"); ok && v != "" {
		cfg.LLM.GLM.Model = v
	}
	if v, ok := os.LookupEnv("GLM_BASE_URL"); ok && v != "" {
		cfg.LLM.GLM.URL = v
	}
}
