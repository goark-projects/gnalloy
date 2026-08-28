package cors

import (
	"strings"
	"time"
)

var defaultMethods = []string{"GET", "HEAD", "POST"}

// Config 描述 CORS 策略。
type Config struct {
	AllowAnyOrigin   bool
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	ExposedHeaders   []string
	AllowAnyHeader   bool
	AllowCredentials bool
	MaxAge           time.Duration
	ShortCircuit     bool
}

type normalizedConfig struct {
	allowAnyOrigin   bool
	allowedOrigins   map[string]struct{}
	allowedMethods   []string
	methodSet        map[string]struct{}
	allowedHeaders   []string
	exposedHeaders   []string
	allowAnyHeader   bool
	allowCredentials bool
	maxAge           time.Duration
	shortCircuit     bool
}

func normalizeConfig(cfg Config) (normalizedConfig, error) {
	out := normalizedConfig{
		allowAnyOrigin:   cfg.AllowAnyOrigin,
		allowedOrigins:   make(map[string]struct{}, len(cfg.AllowedOrigins)),
		allowAnyHeader:   cfg.AllowAnyHeader,
		allowCredentials: cfg.AllowCredentials,
		maxAge:           cfg.MaxAge,
		shortCircuit:     cfg.ShortCircuit,
	}
	for _, origin := range cfg.AllowedOrigins {
		origin = strings.TrimSpace(origin)
		if origin != "" {
			out.allowedOrigins[origin] = struct{}{}
		}
	}
	if !out.allowAnyOrigin && len(out.allowedOrigins) == 0 {
		return normalizedConfig{}, ErrInvalidConfig
	}
	methods := cfg.AllowedMethods
	if len(methods) == 0 {
		methods = defaultMethods
	}
	out.allowedMethods = normalizeTokens(methods, true)
	if len(out.allowedMethods) == 0 {
		return normalizedConfig{}, ErrInvalidConfig
	}
	out.methodSet = make(map[string]struct{}, len(out.allowedMethods))
	for _, method := range out.allowedMethods {
		out.methodSet[method] = struct{}{}
	}
	out.allowedHeaders = normalizeTokens(cfg.AllowedHeaders, false)
	out.exposedHeaders = normalizeTokens(cfg.ExposedHeaders, false)
	return out, nil
}

func normalizeTokens(values []string, upper bool) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if upper {
			value = strings.ToUpper(value)
		}
		out = append(out, value)
	}
	return out
}
