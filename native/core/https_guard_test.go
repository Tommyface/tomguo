package core

import (
	"net/http"
	"net/url"
	"testing"
)

func TestRequireHTTPS(t *testing.T) {
	valid := []string{
		"https://cdn.example.com/a.json",
		"HTTPS://CDN.EXAMPLE.COM/a.json",
	}
	for _, value := range valid {
		if err := requireHTTPS(value); err != nil {
			t.Errorf("%s 应通过：%v", value, err)
		}
	}
	invalid := []string{
		"http://cdn.example.com/a.json",
		"ftp://cdn.example.com/a.json",
		"//cdn.example.com/a.json",
		"https:///a.json",
		"not a url",
		"",
	}
	for _, value := range invalid {
		if err := requireHTTPS(value); err == nil {
			t.Errorf("%s 应被拒绝", value)
		}
	}
}

func TestRedirectGuard(t *testing.T) {
	req, _ := url.Parse("http://evil.example.com/a.zip")
	if err := registryHTTPClient.CheckRedirect(&http.Request{URL: req}, nil); err == nil {
		t.Fatal("HTTPS → HTTP 重定向应被拒绝")
	}
	req, _ = url.Parse("https://cdn.example.com/a.zip")
	if err := registryHTTPClient.CheckRedirect(&http.Request{URL: req}, nil); err != nil {
		t.Fatalf("HTTPS 重定向应通过：%v", err)
	}
}
