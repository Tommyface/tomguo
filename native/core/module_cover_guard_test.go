package core

import (
	"os"
	"testing"
)

func TestModuleCoverHostAllowed(t *testing.T) {
	whitelist := map[string]bool{"pic.example.com": true, "cdn.example.com": true}
	cases := []struct {
		url  string
		want bool
	}{
		{"https://pic.example.com/a.jpg", true},
		{"http://cdn.example.com/a.jpg", true},
		{"https://evil.com/a.jpg", false},
		{"https://PIC.EXAMPLE.COM/a.jpg", true},
		{"ftp://pic.example.com/a.jpg", false},
		{"http://localhost/a.jpg", false},
		{"http://127.0.0.1/a.jpg", false},
		{"http://192.168.1.1/a.jpg", false},
		{"http://[::1]/a.jpg", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		if got := moduleCoverHostAllowed(tc.url, whitelist); got != tc.want {
			t.Errorf("%s → %v，期望 %v", tc.url, got, tc.want)
		}
	}
}

func TestModuleCoverHostAllowedNoWhitelist(t *testing.T) {
	if !moduleCoverHostAllowed("http://localhost/x", nil) {
		t.Fatal("白名单为空应放行")
	}
}

func TestAllowedModuleCoverHosts(t *testing.T) {
	os.Unsetenv("TOMGUO_COVER_HOSTS")
	if got := allowedModuleCoverHosts(); len(got) != 0 {
		t.Fatalf("未设置应为空：%v", got)
	}
	os.Setenv("TOMGUO_COVER_HOSTS", " PIC.Example.com , cdn.example.com, ")
	defer os.Unsetenv("TOMGUO_COVER_HOSTS")
	got := allowedModuleCoverHosts()
	if !got["pic.example.com"] || !got["cdn.example.com"] || len(got) != 2 {
		t.Fatalf("解析错误：%v", got)
	}
}
