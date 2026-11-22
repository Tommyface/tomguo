package core

import (
	"net/url"
	"os"
	"strings"
)

// moduleCoverHostAllowed 检查 URL 的 host 是否在白名单内。
// 白名单为空表示不启用校验；localhost 与 IP 字面量一律拒绝。
func moduleCoverHostAllowed(raw string, allowed map[string]bool) bool {
	if len(allowed) == 0 {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" || host == "localhost" || host == "::1" || parsed.Hostname() == "0.0.0.0" {
		return false
	}
	if isIPLiteral(host) {
		return false
	}
	return allowed[host]
}

// allowedModuleCoverHosts 读取 TOMGUO_COVER_HOSTS 环境变量（逗号分隔）。
// 未设置时返回空 map，表示不启用白名单，保持现有行为。
func allowedModuleCoverHosts() map[string]bool {
	raw := strings.TrimSpace(os.Getenv("TOMGUO_COVER_HOSTS"))
	if raw == "" {
		return map[string]bool{}
	}
	out := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		host := strings.ToLower(strings.TrimSpace(item))
		if host != "" {
			out[host] = true
		}
	}
	return out
}

func isIPLiteral(host string) bool {
	if len(host) == 0 {
		return false
	}
	if host[0] >= '0' && host[0] <= '9' {
		dots := 0
		for _, symbol := range host {
			if symbol == '.' {
				dots++
			} else if symbol < '0' || symbol > '9' {
				return false
			}
		}
		return dots == 3
	}
	return strings.Contains(host, ":")
}
