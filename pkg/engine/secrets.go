package engine

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)
var keyPattern = regexp.MustCompile(`(?:sb_(?:secret|publishable)_|sbp_)[A-Za-z0-9_-]+`)
var credentialURLPattern = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/@]+:[^\s/@]+@`)
var assignmentPattern = regexp.MustCompile(`(?im)((?:[A-Za-z0-9_]*(?:key|token|secret|password)[A-Za-z0-9_]*|authorization|cookie|set-cookie)["']?\s*[:=]\s*)[^\r\n]+`)

func Redact(text string) string {
	text = jwtPattern.ReplaceAllString(text, "[REDACTED]")
	text = keyPattern.ReplaceAllString(text, "[REDACTED]")
	text = credentialURLPattern.ReplaceAllString(text, "${1}[REDACTED]@")
	return assignmentPattern.ReplaceAllString(text, "${1}[REDACTED]")
}
func PublicVariable(name string) bool {
	for _, prefix := range []string{"VITE_", "NEXT_PUBLIC_", "NUXT_PUBLIC_", "PUBLIC_", "REACT_APP_", "EXPO_PUBLIC_", "VUE_APP_", "GATSBY_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
func Privileged(source string) bool {
	source = strings.ToUpper(source)
	for _, word := range []string{"SECRET", "SERVICE_ROLE", "DB_URL", "PASSWORD"} {
		if strings.Contains(source, word) {
			return true
		}
	}
	return false
}
func PrivilegedValue(value string) bool {
	if strings.HasPrefix(value, "sb_secret_") || strings.HasPrefix(value, "sbp_") {
		return true
	}
	if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
		if _, present := parsed.User.Password(); present {
			return true
		}
	}
	parts := strings.Split(value, ".")
	if len(parts) > 1 {
		payload, err := base64.RawURLEncoding.DecodeString(parts[1])
		claims := map[string]any{}
		if err == nil && json.Unmarshal(payload, &claims) == nil {
			return claims["role"] == "service_role"
		}
	}
	return false
}
func redactJSON(value any) any {
	switch data := value.(type) {
	case string:
		return Redact(data)
	case []any:
		for index, item := range data {
			data[index] = redactJSON(item)
		}
	case map[string]any:
		for key, item := range data {
			if Privileged(key) || strings.Contains(strings.ToUpper(key), "TOKEN") {
				data[key] = "[REDACTED]"
			} else {
				data[key] = redactJSON(item)
			}
		}
	}
	return value
}
