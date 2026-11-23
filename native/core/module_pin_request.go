package core

import (
	"net/http"
	"time"
)

type pinnedHostKey struct{}

func (d *Downloader) doPinnedRequest(request *http.Request, timeout time.Duration, host string) (*http.Response, error) {
	address := request.URL.Host
	request.URL.Host = host
	request.Host = host
	defer func() {
		request.URL.Host = address
		request.Host = address
	}()
	ip := host
	if pinned, ok := request.Context().Value(pinnedAddressKey{}).(string); ok && pinned != "" {
		ip = pinned
	}
	client := modulePinnedClient(ip, host)
	if timeout > 0 {
		client.Timeout = timeout
	}
	return client.Do(request)
}

type pinnedAddressKey struct{}
