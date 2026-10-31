// Package env fornece helpers pra ler variaveis de ambiente com fallback.
package env

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// GetOrDefault retorna o valor da env var ou o fallback se vazia.
func GetOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// GetIntOrDefault retorna o valor inteiro da env var ou o fallback se
// estiver vazia ou nao for um numero valido.
func GetIntOrDefault(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

// GetBoolOrDefault aceita true/false, 1/0, yes/no (strconv.ParseBool).
func GetBoolOrDefault(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

// GetDurationOrDefault lê durações no formato do Go (500ms, 30s, 72h).
func GetDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

// GetListOrDefault lê uma lista separada por vírgulas, ignorando itens vazios.
func GetListOrDefault(key string, fallback []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}
