package core

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// 站源模块的「钉 IP」请求栈。
//
// 直接把 URL 里的域名换成 IP 会让 TLS 拿不到 SNI，Cloudflare 一类站点会
// 直接 handshake failure。正确做法是保持 URL 与 SNI 不变，只在拨号阶段
// 连到指定 IP，所以这里给每个 (IP, 域名) 组合缓存一个专用 Transport。

var modulePinnedClients sync.Map // key: "<ip>|<域名>" -> *http.Client

func modulePinnedClient(ip, serverName string) *http.Client {
	key := ip + "|" + serverName
	if cached, ok := modulePinnedClients.Load(key); ok {
		return cached.(*http.Client)
	}
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			_, port, err := net.SplitHostPort(address)
			if err != nil || port == "" {
				port = "443"
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		},
		TLSClientConfig:     &tls.Config{ServerName: serverName},
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 15 * time.Second,
		IdleConnTimeout:     60 * time.Second,
		MaxIdleConnsPerHost: 4,
	}
	client := &http.Client{Transport: transport}
	modulePinnedClients.Store(key, client)
	return client
}
