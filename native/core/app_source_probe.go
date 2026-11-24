package core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const nativeProbeTimeout = 2 * time.Second
const nativeProbeTTL = 10 * time.Minute

type nativeSourceProbe struct {
	Source    string    `json:"source"`
	Reachable bool      `json:"reachable"`
	Status    int       `json:"status,omitempty"`
	ElapsedMS int64     `json:"elapsedMs"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checkedAt"`
}

func (probe nativeSourceProbe) stale() bool {
	return probe.CheckedAt.IsZero() || time.Since(probe.CheckedAt) > nativeProbeTTL
}

// probeAddresses 取判活用的入口地址列表：插件源按 manifest 里的域名顺序，
// 内置源用已解析的 provider 基础地址。任一入口可达即视为可用。
func (engine *nativeEngine) probeAddresses(source string) []string {
	addresses := make([]string, 0, 4)
	seen := map[string]bool{}
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			raw = "https://" + raw
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return
		}
		address := parsed.Scheme + "://" + parsed.Host + "/"
		if seen[address] {
			return
		}
		seen[address] = true
		addresses = append(addresses, address)
	}
	if module := moduleSource(source); module != nil {
		for _, host := range module.manifest.Hosts {
			add(host)
		}
	}
	if len(addresses) == 0 && engine.downloader != nil {
		add(strings.TrimRight(engine.downloader.providerBaseURL(source), "/") + "/")
	}
	return addresses
}

func (engine *nativeEngine) probeSource(ctx context.Context, source string) nativeSourceProbe {
	source = canonicalProviderSource(source)
	probe := nativeSourceProbe{Source: source, CheckedAt: time.Now()}
	addresses := engine.probeAddresses(source)
	if len(addresses) == 0 {
		probe.Error = "站源没有可用入口地址"
		return probe
	}
	var client *http.Client
	if engine.downloader != nil {
		client = engine.downloader.client
	}
	if client == nil {
		client = &http.Client{Timeout: nativeProbeTimeout}
	}
	start := time.Now()
	for _, address := range addresses {
		probeCtx, cancel := context.WithTimeout(ctx, nativeProbeTimeout)
		request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, address, nil)
		if err != nil {
			cancel()
			probe.Error = "站源入口地址无效"
			continue
		}
		request.Header.Set("User-Agent", userAgent)
		request.Header.Set("Range", "bytes=0-1023")
		response, err := client.Do(request)
		if err != nil {
			cancel()
			probe.Error = publicError(err).Error()
			continue
		}
		_, _ = io.CopyN(io.Discard, response.Body, 1024)
		response.Body.Close()
		cancel()
		probe.Status = response.StatusCode
		if response.StatusCode > 0 && response.StatusCode < 500 {
			probe.Reachable = true
			probe.Error = ""
			break
		}
		probe.Error = "站源入口返回 HTTP " + strings.TrimSpace(http.StatusText(response.StatusCode))
	}
	probe.ElapsedMS = time.Since(start).Milliseconds()
	return probe
}

func (engine *nativeEngine) probeSources(ctx context.Context, sources []string, force bool) []nativeSourceProbe {
	if len(sources) == 0 {
		seen := map[string]bool{}
		for _, manifest := range bundledModules {
			if manifest.Retired || seen[manifest.ID] {
				continue
			}
			seen[manifest.ID] = true
			sources = append(sources, manifest.ID)
		}
		for _, manifest := range ModuleManifests() {
			if seen[manifest.ID] {
				continue
			}
			seen[manifest.ID] = true
			sources = append(sources, manifest.ID)
		}
	}
	engine.mu.Lock()
	if engine.sourceProbes == nil {
		engine.sourceProbes = map[string]nativeSourceProbe{}
	}
	pending := make([]string, 0, len(sources))
	results := make([]nativeSourceProbe, 0, len(sources))
	for _, source := range sources {
		key := canonicalProviderSource(source)
		if key == "" {
			continue
		}
		if cached, found := engine.sourceProbes[key]; found && !force && !cached.stale() {
			results = append(results, cached)
			continue
		}
		pending = append(pending, key)
	}
	engine.mu.Unlock()

	probes := make([]nativeSourceProbe, len(pending))
	var wait sync.WaitGroup
	slots := make(chan struct{}, 6)
	for index, source := range pending {
		wait.Add(1)
		go func(index int, source string) {
			defer wait.Done()
			slots <- struct{}{}
			probes[index] = engine.probeSource(ctx, source)
			<-slots
		}(index, source)
	}
	wait.Wait()

	engine.mu.Lock()
	for _, probe := range probes {
		engine.sourceProbes[probe.Source] = probe
		results = append(results, probe)
	}
	engine.mu.Unlock()
	return results
}

// sourceReachable 返回缓存的探活结论；没有结论时按可用处理，避免误藏。
func (engine *nativeEngine) sourceReachable(source string) (bool, string) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	probe, found := engine.sourceProbes[canonicalProviderSource(source)]
	if !found || probe.stale() {
		return true, ""
	}
	if probe.Reachable {
		return true, ""
	}
	message := probe.Error
	if message == "" {
		message = "站源入口不可达"
	}
	return false, message
}

var errSourceRetired = errors.New("该站源已停用")
