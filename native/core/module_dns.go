package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DoH 服务：本地 DNS 被污染时用来拿真实地址。
var moduleDoHEndpoints = []string{
	"https://doh.pub/dns-query",
	"https://dns.alidns.com/resolve",
	"https://cloudflare-dns.com/dns-query",
}

type moduleDoHAnswer struct {
	Status int `json:"Status"`
	Answer []struct {
		Type int    `json:"type"`
		Data string `json:"data"`
	} `json:"Answer"`
}

// resolveModuleDNS 通过 DoH 解析域名，返回全部 A 记录。
func resolveModuleDNS(ctx context.Context, name string) ([]string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("域名不能为空")
	}
	if ip := net.ParseIP(name); ip != nil {
		return []string{name}, nil
	}
	client := &http.Client{Timeout: 8 * time.Second}
	var lastErr error
	for _, endpoint := range moduleDoHEndpoints {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?name="+name+"&type=A", nil)
		if err != nil {
			continue
		}
		request.Header.Set("Accept", "application/dns-json")
		request.Header.Set("User-Agent", userAgent)
		response, err := client.Do(request)
		if err != nil {
			lastErr = err
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<16))
		response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		var answer moduleDoHAnswer
		if json.Unmarshal(raw, &answer) != nil {
			lastErr = errors.New("DoH 返回无法解析")
			continue
		}
		var out []string
		for _, entry := range answer.Answer {
			if entry.Type != 1 {
				continue
			}
			if net.ParseIP(entry.Data) != nil {
				out = append(out, entry.Data)
			}
		}
		if len(out) > 0 {
			return out, nil
		}
		lastErr = errors.New("DoH 没有返回 A 记录")
	}
	if lastErr == nil {
		lastErr = errors.New("DoH 全部不可用")
	}
	return nil, lastErr
}
