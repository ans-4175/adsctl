package graph

import (
	"regexp"
	"strings"
)

var secretQuery = regexp.MustCompile(`(?i)(access_token|appsecret_proof|client_secret|app_secret)=([^&\s"<>]+)`)

func secretKey(key string) bool {
	switch strings.ToLower(key) {
	case "access_token", "token", "app_secret", "client_secret", "appsecret_proof", "refresh_token":
		return true
	}
	return false
}
func (c *Client) sanitize(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if secretKey(k) {
				x[k] = "[REDACTED]"
			} else {
				x[k] = c.sanitize(value)
			}
		}
		return x
	case []any:
		for i, value := range x {
			x[i] = c.sanitize(value)
		}
		return x
	case string:
		return secretQuery.ReplaceAllString(c.Redact(x), "$1=[REDACTED]")
	default:
		return v
	}
}
