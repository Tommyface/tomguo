package core

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestModuleReadRetriesThroughDoHWhenDirectFails(t *testing.T) {
	var direct, lookups atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Host != "app.example.test" || request.TLS.ServerName != "app.example.test" {
			t.Error("module retry changed host or SNI", request.Host, request.TLS.ServerName)
		}
		_, _ = writer.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == "app.example.test:443" {
			direct.Add(1)
			return nil, io.EOF
		}
		if address != "93.184.216.34:443" {
			t.Error("unexpected module destination", address)
			return nil, io.EOF
		}
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	resolver := newDNSResolver(base)
	resolver.client.Transport = sourceFixtureTransport(func(request *http.Request) (*http.Response, error) {
		lookups.Add(1)
		return sourceFixtureResponse(request, 200, `{"Status":0,"Answer":[{"type":1,"TTL":120,"data":"93.184.216.34"}]}`), nil
	})
	transport := newCDNTransport(base, resolver)
	defer transport.CloseIdleConnections()

	request, _ := http.NewRequest(http.MethodGet, "https://app.example.test/cloud/v2/theater/home_page?page_num=1", nil)
	response, err := transport.RoundTrip(withModuleResolveFallback(request))
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != `{"ok":true}` {
		t.Fatalf("body = %q", body)
	}
	if direct.Load() != 1 || lookups.Load() != 1 {
		t.Fatalf("direct=%d lookups=%d", direct.Load(), lookups.Load())
	}
}

func TestModulePostWithoutReplayIsNotMarked(t *testing.T) {
	request, _ := http.NewRequest(http.MethodPost, "https://app.example.test/cloud/v3/search", nil)
	marked := withModuleResolveFallback(request)
	if marked.Context().Value(moduleResolveFallbackKey{}) != nil {
		t.Fatal("POST must not be marked for replay")
	}
	getRequest, _ := http.NewRequest(http.MethodGet, "https://app.example.test/cloud/v2/theater", nil)
	if withModuleResolveFallback(getRequest).Context().Value(moduleResolveFallbackKey{}) != true {
		t.Fatal("GET should be marked")
	}
}
