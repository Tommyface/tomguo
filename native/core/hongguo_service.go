package core

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"math/bits"
	"regexp"
	"sort"
	"unicode"
	"unicode/utf8"
	"golang.org/x/text/unicode/norm"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const hongguoAppBaseURL = "https://api5-normal-sinfonlineb.fqnovel.com"

const hongguoAppUserAgent = "com.phoenix.read/73532 (Linux; U; Android 16; zh_CN; 25053RT47C; Build/BP2A.250605.031.A3; Cronet/TTNetVersion:04657795 2026-01-23 QuicVersion:c67e9834 2025-09-08)"

type hongguoCatalogCursor struct {
	Offset        int       `json:"offset"`
	SessionID     string    `json:"sessionId,omitempty"`
	LastID        string    `json:"lastId,omitempty"`
	PageSignature string    `json:"pageSignature,omitempty"`
	Initialized   bool      `json:"initialized"`
	Exhausted     bool      `json:"exhausted"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type hongguoCatalogState struct {
	Version   int                             `json:"version"`
	DeviceID  string                          `json:"deviceId"`
	InstallID string                          `json:"installId"`
	Feeds     map[string]hongguoCatalogCursor `json:"feeds"`
}

type hongguoAppClient struct {
	mu             sync.Mutex
	catalogMu      sync.Mutex
	baseURL        string
	state          hongguoCatalogState
	details        map[string]hongguoDetailEntry
	pending        map[string]*hongguoDetailCall
	searches       map[string]hongguoSearchEntry
	searchPending  map[string]*hongguoSearchCall
	suggestions    map[string]hongguoSuggestionEntry
	suggestPending map[string]*hongguoSuggestionCall
	danmaku        map[string]hongguoDanmakuCacheEntry
	danmakuPending map[string]*hongguoDanmakuCall
}

func (downloader *Downloader) hongguoClient() *hongguoAppClient {
	downloader.hongguoOnce.Do(func() {
		downloader.hongguo = &hongguoAppClient{
			baseURL: hongguoAppBaseURL,
			state:   hongguoCatalogState{Version: 1, DeviceID: newHongguoDeviceID(), InstallID: newHongguoDeviceID(), Feeds: map[string]hongguoCatalogCursor{}},
			details: map[string]hongguoDetailEntry{}, pending: map[string]*hongguoDetailCall{},
			searches: map[string]hongguoSearchEntry{}, searchPending: map[string]*hongguoSearchCall{},
			suggestions: map[string]hongguoSuggestionEntry{}, suggestPending: map[string]*hongguoSuggestionCall{},
		}
	})
	return downloader.hongguo
}

func cloneHongguoCatalogState(state *hongguoCatalogState) *hongguoCatalogState {
	if state == nil {
		return nil
	}
	cloned := *state
	cloned.Feeds = make(map[string]hongguoCatalogCursor, len(state.Feeds))
	for name, cursor := range state.Feeds {
		cloned.Feeds[name] = cursor
	}
	return &cloned
}

func (downloader *Downloader) restoreHongguoCatalog(state *hongguoCatalogState) {
	if state == nil || state.Version != 1 || !hongguoNumericID.MatchString(state.DeviceID) || !hongguoNumericID.MatchString(state.InstallID) {
		return
	}
	cloned := cloneHongguoCatalogState(state)
	for name, cursor := range cloned.Feeds {
		known := false
		for _, genre := range hongguoAppGenres {
			known = known || name == genre.key || name == "category:"+genre.key
		}
		if !known || !cursor.Initialized || cursor.Offset < 0 || cursor.Offset > 1_000_000 || len(cursor.SessionID) > 4096 || len(cursor.LastID) > 256 || len(cursor.PageSignature) > 64 {
			delete(cloned.Feeds, name)
		}
	}
	client := downloader.hongguoClient()
	client.catalogMu.Lock()
	defer client.catalogMu.Unlock()
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.state.Feeds) == 0 {
		client.state = *cloned
	}
}

func (downloader *Downloader) hongguoCatalogSnapshot() *hongguoCatalogState {
	client := downloader.hongguoClient()
	client.mu.Lock()
	defer client.mu.Unlock()
	return cloneHongguoCatalogState(&client.state)
}

func hongguoCatalogInitialized(state *hongguoCatalogState) bool {
	if state != nil && state.Version == 1 {
		for _, cursor := range state.Feeds {
			if cursor.Initialized {
				return true
			}
		}
	}
	return false
}

func hongguoCatalogHasMore(state *hongguoCatalogState) bool {
	for _, genre := range hongguoAppGenres {
		if state == nil || !state.Feeds[genre.key].Exhausted {
			return true
		}
	}
	return false
}

func (downloader *Downloader) hongguoAppRequest(ctx context.Context, method, path string, extra url.Values, payload any) (map[string]any, error) {
	client := downloader.hongguoClient()
	client.mu.Lock()
	base, deviceID, installID := client.baseURL, client.state.DeviceID, client.state.InstallID
	client.mu.Unlock()
	query := url.Values{
		"aid": {"8662"}, "app_name": {"novelread"}, "version_code": {"73532"}, "version_name": {"7.3.5.32"},
		"manifest_version_code": {"73532"}, "update_version_code": {"73532"}, "channel": {"update_64"},
		"device_platform": {"android"}, "os": {"android"}, "ssmix": {"a"}, "device_type": {"25053RT47C"},
		"device_brand": {"Redmi"}, "language": {"zh"}, "os_api": {"36"}, "os_version": {"16"},
		"resolution": {"1280*2772"}, "dpi": {"520"}, "ac": {"wifi"}, "device_id": {deviceID}, "iid": {installID},
	}
	for key, values := range extra {
		query[key] = append([]string(nil), values...)
	}
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, err
		}
	}
	attempts := downloader.cfg.Retries
	if attempts < 1 {
		attempts = 1
	}
	if attempts > 3 {
		attempts = 3
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(time.Duration(attempt) * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		request.Header.Set("User-Agent", hongguoAppUserAgent)
		request.Header.Set("Accept", "application/json")
		request.Header.Set("X-XS-From-Web", "0")
		request.Header.Set("Sdk-Version", "2")
		comment, _ := ctx.Value(hongguoCommentKey{}).(bool)
		var commentNonce hongguoCommentNonce
		if comment {
			commentNonce, err = newHongguoCommentNonce()
			if err != nil {
				return nil, errors.New("无法准备红果文字请求")
			}
			request.Header.Set("Comment-Source", "601")
			request.Header.Set("Server-Channel", "1000")
		}
		if payload != nil {
			request.Header.Set("Content-Type", "application/json; charset=utf-8")
		}
		response, err := downloader.doPreparedCatalogRequest(request, 20*time.Second, func(prepared *http.Request) {
			now := time.Now()
			query.Set("_rticket", strconv.FormatInt(now.UnixMilli(), 10))
			prepared.URL.RawQuery = query.Encode()
			if comment {
				signHongguoCommentRequest(prepared, commentNonce, now)
			} else {
				signHongguoRequest(prepared, body, now)
			}
		})
		if err != nil {
			lastErr = err
			var backoff *requestBackoff
			if errors.As(err, &backoff) || ctx.Err() != nil {
				return nil, err
			}
			continue
		}
		content, readErr := io.ReadAll(io.LimitReader(response.Body, providerMaxBodyBytes+1))
		response.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if response.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("红果 App 接口 HTTP %d", response.StatusCode)
			if response.StatusCode >= 400 && response.StatusCode < 500 {
				return nil, lastErr
			}
			continue
		}
		if len(content) == 0 || len(content) > providerMaxBodyBytes {
			return nil, errors.New("红果 App 接口未返回有效数据")
		}
		var result map[string]any
		decoder := json.NewDecoder(bytes.NewReader(content))
		decoder.UseNumber()
		if err := decoder.Decode(&result); err != nil || result == nil {
			return nil, errors.New("红果 App 接口返回格式异常")
		}
		code := firstNonEmpty(mapString(result, "code", "Code", "status_code"), mapString(nestedMap(result, "BaseResp"), "StatusCode"))
		if code != "" && code != "0" {
			return nil, fmt.Errorf("红果 App 接口暂不可用（%s）", truncate(code, 20))
		}
		return result, nil
	}
	return nil, lastErr
}

type hongguoRecommendationQuery struct {
	Genre     string   `json:"genre"`
	Offset    int      `json:"offset"`
	SessionID string   `json:"sessionId"`
	Seen      []string `json:"seen"`
}
type hongguoRecommendationPage struct {
	Dramas     []Drama `json:"data"`
	NextOffset int     `json:"nextOffset"`
	SessionID  string  `json:"sessionId"`
	HasMore    bool    `json:"hasMore"`
	Saved      bool    `json:"saved"`
}
func (query hongguoRecommendationQuery) validate() error {
	known := false
	for _, genre := range hongguoAppGenres {
		known = known || query.Genre == genre.key
	}
	if !known || query.Offset < 0 || query.Offset > 1_000_000 || len(query.SessionID) > 4096 || strings.ContainsAny(query.SessionID, "\r\n\x00") || len(query.Seen) > 540 {
		return errors.New("推荐分类或分页参数无效，请重新获取")
	}
	for _, id := range query.Seen {
		if !hongguoNumericID.MatchString(strings.TrimPrefix(id, "hongguo:")) {
			return errors.New("推荐分页含无效剧集 ID")
		}
	}
	return nil
}
func (downloader *Downloader) fetchHongguoRecommendations(ctx context.Context, query hongguoRecommendationQuery) (hongguoRecommendationPage, error) {
	page := hongguoRecommendationPage{Dramas: []Drama{}}
	if err := query.validate(); err != nil {
		return page, err
	}
	scene, category := "", ""
	for _, genre := range hongguoAppGenres {
		if genre.key == query.Genre {
			scene, category = genre.scene, genre.name
		}
	}
	seen := make(map[string]bool, len(query.Seen))
	filterIDs := make([]string, 0, len(query.Seen))
	for _, id := range query.Seen {
		id = strings.TrimPrefix(id, "hongguo:")
		if !seen[id] {
			filterIDs = append(filterIDs, id)
			seen[id] = true
		}
	}
	payload := map[string]any{
		"req_scene": scene, "offset": query.Offset, "limit": 18,
		"req_type": "only_content", "need_selector_panel": false, "client_req_type": 3,
		"session_id": query.SessionID, "filter_ids": strings.Join(filterIDs, ","),
		"select_items": map[string]any{
			"genre": []string{query.Genre}, "sort": []string{}, "gender": []string{},
			"category_dim_theme": []string{}, "category_dim_role": []string{}, "category_dim_epoch": []string{},
			"online_time": []string{}, "creation_status": []string{},
		},
	}
	if query.Offset > 0 {
		payload["client_req_type"] = 2
	}
	result, err := downloader.hongguoAppRequest(ctx, http.MethodPost, "/reading/distribution/category/landpage/v/", nil, payload)
	if err != nil {
		return page, err
	}
	data := nestedMap(result, "data")
	rows, valid := data["video_data"].([]any)
	hasMore, paginationOK := data["has_more"].(bool)
	next, parseErr := strconv.Atoi(mapString(data, "next_offset"))
	if !valid || !paginationOK || hasMore && parseErr != nil {
		return page, errors.New("红果推荐数据格式异常，已保留上次位置")
	}
	validRows := 0
	for _, row := range rows {
		drama := hongguoDramaFromAny(row, category)
		if drama.ID != "" {
			validRows++
		}
		id := strings.TrimPrefix(drama.ID, "hongguo:")
		if drama.ID == "" || seen[id] {
			continue
		}
		seen[id] = true
		page.Dramas = append(page.Dramas, drama)
	}
	if len(rows) > 0 && validRows == 0 {
		return hongguoRecommendationPage{}, errors.New("红果推荐未返回可识别的剧集")
	}
	if hasMore && (next <= query.Offset || next > 1_000_000 || len(page.Dramas) == 0) {
		return hongguoRecommendationPage{}, errors.New("红果推荐分页未前进，可重试或重新获取")
	}
	page.NextOffset, page.HasMore, page.SessionID = next, hasMore, mapString(data, "session_id")
	if !hasMore && (next < query.Offset || next > 1_000_000) {
		page.NextOffset = query.Offset
	}
	if len(page.SessionID) > 4096 || strings.ContainsAny(page.SessionID, "\r\n\x00") {
		return hongguoRecommendationPage{}, errors.New("红果推荐分页标记无效")
	}
	return page, ctx.Err()
}

type hongguoSearchSuggestion struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

func (d *Downloader) hongguoSearchSuggestions(ctx context.Context, query string) ([]hongguoSearchSuggestion, error) {
	query, err := hongguoSearchKeyword(query)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, hongguoSuggestionTimeout)
	defer cancel()
	client := d.hongguoClient()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client.mu.Lock()
		if cached, found := client.suggestions[query]; found && time.Now().Before(cached.ExpiresAt) {
			items := append([]hongguoSearchSuggestion{}, cached.Items...)
			client.mu.Unlock()
			return items, nil
		}
		if pending := client.suggestPending[query]; pending != nil {
			client.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-pending.done:

				if errors.Is(pending.err, context.Canceled) || errors.Is(pending.err, context.DeadlineExceeded) {
					continue
				}
				return append([]hongguoSearchSuggestion{}, pending.items...), pending.err
			}
		}
		if len(client.suggestPending) >= hongguoSuggestionPendingLimit {
			client.mu.Unlock()
			return nil, errors.New("红果搜索联想请求较多，请稍后重试")
		}
		pending := &hongguoSuggestionCall{done: make(chan struct{})}
		client.suggestPending[query] = pending
		client.mu.Unlock()

		items, err := d.fetchHongguoSearchSuggestions(ctx, query)
		client.mu.Lock()
		if err == nil {
			now := time.Now()
			oldestKey, oldestTime := "", now.Add(hongguoSuggestionTTL)
			for key, entry := range client.suggestions {
				if !now.Before(entry.ExpiresAt) {
					delete(client.suggestions, key)
				} else if entry.ExpiresAt.Before(oldestTime) {
					oldestKey, oldestTime = key, entry.ExpiresAt
				}
			}
			if len(client.suggestions) >= hongguoSuggestionCacheLimit {
				delete(client.suggestions, oldestKey)
			}
			client.suggestions[query] = hongguoSuggestionEntry{Items: items, ExpiresAt: now.Add(hongguoSuggestionTTL)}
		}
		pending.items, pending.err = items, err
		delete(client.suggestPending, query)
		close(pending.done)
		client.mu.Unlock()
		return append([]hongguoSearchSuggestion{}, items...), err
	}
}



const (
	hongguoSuggestionLimit        = 10
	hongguoSuggestionCacheLimit   = 128
	hongguoSuggestionPendingLimit = 32
	hongguoSuggestionBodyLimit    = 256 * 1024
	hongguoSuggestionTimeout      = 5 * time.Second
	hongguoSuggestionTTL          = 2 * time.Minute
)


type hongguoSuggestionRecord struct {
	Name      string         `json:"name"`
	WordType  string         `json:"word_type"`
	Keyword   any            `json:"keyword"`
	VideoData map[string]any `json:"video_data"`
}

type hongguoSuggestionEntry struct {
	Items     []hongguoSearchSuggestion
	ExpiresAt time.Time
}

type hongguoSuggestionCall struct {
	done  chan struct{}
	items []hongguoSearchSuggestion
	err   error
}


func (d *Downloader) fetchHongguoSearchSuggestions(ctx context.Context, query string) ([]hongguoSearchSuggestion, error) {
	records, err := d.fetchHongguoSuggestionRecords(ctx, query, hongguoSuggestionLimit)
	if err != nil {
		return nil, err
	}
	items := make([]hongguoSearchSuggestion, 0, hongguoSuggestionLimit)
	seen := make(map[string]bool)
	for _, item := range records {
		name, err := hongguoSearchKeyword(item.Name)
		key := strings.ToLower(name)
		if err != nil || seen[key] {
			continue
		}
		seen[key] = true
		kind := item.WordType
		switch kind {
		case "short_play_name", "short_play_category", "common_query", "actor_name", "short_play_actor":
		default:
			kind = ""
		}
		items = append(items, hongguoSearchSuggestion{Name: name, Type: kind})
		if len(items) == hongguoSuggestionLimit {
			break
		}
	}
	return items, nil
}

func (d *Downloader) fetchHongguoSuggestionRecords(ctx context.Context, query string, count int) ([]hongguoSuggestionRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, hongguoSuggestionTimeout)
	defer cancel()
	params := url.Values{"app_id": {"8662"}, "query": {query}, "count": {strconv.Itoa(count)}}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, hongguoBaseURL+"/incent_resource/suggestion?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Referer", hongguoBaseURL+"/")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")

	response, err := d.doCatalogRequestWithTimeout(request, hongguoSuggestionTimeout)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, hongguoSuggestionBodyLimit+1))
	if err != nil {
		return nil, err
	}
	if len(body) > hongguoSuggestionBodyLimit {
		return nil, errors.New("红果搜索联想响应过大")
	}
	if response.StatusCode != http.StatusOK || catalogResponseBlockReason(response, body) != "" {
		return nil, d.catalogResponseError(request, response, body)
	}
	var result struct {
		Items []hongguoSuggestionRecord `json:"suggest_list"`
		Data  struct {
			Items []hongguoSuggestionRecord `json:"suggest_list"`
		} `json:"data"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, errors.New("红果搜索联想未返回有效数据")
	}
	if result.Items == nil {
		result.Items = result.Data.Items
	}
	if result.Items == nil {
		return nil, errors.New("红果搜索联想未返回有效数据")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, errors.New("红果搜索联想未返回有效数据")
	}
	return result.Items, nil
}

// --- migrated from provider_hongguo_search.go ---
const (
	hongguoSearchTimeout      = 40 * time.Second
	hongguoSearchPageTimeout  = 12 * time.Second
	hongguoSearchPendingLimit = 32
	hongguoSearchNameCount    = 50
)

type hongguoSearchEntry struct {
	Dramas    []Drama
	Total     int
	Limited   bool
	Warning   string
	ExpiresAt time.Time
}

type hongguoSearchCall struct {
	done     chan struct{}
	changed  chan struct{}
	entry    hongguoSearchEntry
	snapshot hongguoSearchEntry
	revision uint64
	err      error
}

type hongguoSearchProgressKey struct{}

func reportHongguoSearchProgress(ctx context.Context, entry hongguoSearchEntry) {
	if progress, ok := ctx.Value(hongguoSearchProgressKey{}).(func(hongguoSearchEntry)); ok && len(entry.Dramas) > 0 {
		entry.Total = max(entry.Total, len(entry.Dramas))
		progress(entry)
	}
}

func hongguoSearchKeyword(keyword string) (string, error) {
	keyword = norm.NFKC.String(strings.TrimSpace(keyword))
	if keyword == "" || !utf8.ValidString(keyword) || utf8.RuneCountInString(keyword) > 80 || strings.IndexFunc(keyword, unicode.IsControl) >= 0 {
		return "", errors.New("请输入 1 至 80 个字符的搜索词")
	}
	return keyword, nil
}

func (downloader *Downloader) searchHongguoDramas(ctx context.Context, keyword string) (hongguoSearchEntry, error) {
	return downloader.searchHongguoDramasProgress(ctx, keyword, nil)
}

func (downloader *Downloader) searchHongguoDramasProgress(ctx context.Context, keyword string, progress func(hongguoSearchEntry)) (hongguoSearchEntry, error) {
	keyword, err := hongguoSearchKeyword(keyword)
	if err != nil {
		return hongguoSearchEntry{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, hongguoSearchTimeout)
	defer cancel()
	client := downloader.hongguoClient()
	for {
		if err := ctx.Err(); err != nil {
			return hongguoSearchEntry{}, err
		}
		client.mu.Lock()
		if cached, found := client.searches[keyword]; found && time.Now().Before(cached.ExpiresAt) {
			client.mu.Unlock()
			return cloneHongguoSearchEntry(cached), nil
		}
		if pending := client.searchPending[keyword]; pending != nil {
			client.mu.Unlock()
			entry, err := downloader.waitHongguoSearch(ctx, pending, progress)
			if (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) && ctx.Err() == nil {
				continue
			}
			return entry, err
		}
		if len(client.searchPending) >= hongguoSearchPendingLimit {
			client.mu.Unlock()
			return hongguoSearchEntry{}, errors.New("红果搜索请求较多，请稍后重试")
		}
		pending := &hongguoSearchCall{done: make(chan struct{}), changed: make(chan struct{})}
		client.searchPending[keyword] = pending
		client.mu.Unlock()
		fetchCtx := context.WithValue(ctx, hongguoSearchProgressKey{}, func(entry hongguoSearchEntry) {
			client.mu.Lock()
			pending.snapshot = cloneHongguoSearchEntry(entry)
			pending.revision++
			close(pending.changed)
			pending.changed = make(chan struct{})
			client.mu.Unlock()
			if progress != nil {
				progress(cloneHongguoSearchEntry(entry))
			}
		})
		entry, err := downloader.fetchHongguoSearch(fetchCtx, keyword)
		client.mu.Lock()

		if err == nil && entry.Warning == "" {
			if len(client.searches) >= 64 {
				client.searches = map[string]hongguoSearchEntry{}
			}
			entry.ExpiresAt = time.Now().Add(5 * time.Minute)
			client.searches[keyword] = entry
		}
		pending.entry, pending.err = entry, err
		delete(client.searchPending, keyword)
		close(pending.done)
		client.mu.Unlock()
		return cloneHongguoSearchEntry(entry), err
	}
}

func (downloader *Downloader) waitHongguoSearch(ctx context.Context, pending *hongguoSearchCall, progress func(hongguoSearchEntry)) (hongguoSearchEntry, error) {
	client := downloader.hongguoClient()
	var revision uint64
	for {
		client.mu.Lock()
		changed := pending.changed
		var update *hongguoSearchEntry
		if progress != nil && pending.revision > revision {
			entry := cloneHongguoSearchEntry(pending.snapshot)
			update = &entry
			revision = pending.revision
		}
		client.mu.Unlock()
		if update != nil {
			progress(*update)
		}
		select {
		case <-ctx.Done():
			return hongguoSearchEntry{}, ctx.Err()
		case <-pending.done:
			return cloneHongguoSearchEntry(pending.entry), pending.err
		case <-changed:
		}
	}
}

func (downloader *Downloader) fetchHongguoSearch(ctx context.Context, keyword string) (hongguoSearchEntry, error) {

	names, namesErr := downloader.fetchHongguoSearchNames(ctx, keyword)
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) && len(names) > 0 {
			return hongguoSearchEntry{Dramas: names, Total: len(names), Limited: true, Warning: "搜索已到时限，已保留名称匹配结果，可重试"}, nil
		}
		return hongguoSearchEntry{}, err
	}
	reportHongguoSearchProgress(ctx, hongguoSearchEntry{Dramas: mergeHongguoSearchDramas([]Drama{}, names), Limited: true})
	pageCtx, cancel := context.WithTimeout(ctx, hongguoSearchPageTimeout)
	defer cancel()
	page, pageErr := downloader.fetchHongguoSearchPage(pageCtx, keyword)
	if errors.Is(ctx.Err(), context.Canceled) {
		return hongguoSearchEntry{}, ctx.Err()
	}
	if pageErr != nil && namesErr != nil || len(page.Dramas)+len(names) == 0 && (pageErr != nil || namesErr != nil) {
		return hongguoSearchEntry{}, errors.Join(pageErr, namesErr)
	}
	entry := hongguoSearchEntry{Dramas: make([]Drama, 0, len(page.Dramas)+len(names)), Total: page.Total, Limited: page.Limited}
	for _, batch := range [][]Drama{page.Dramas, names} {
		entry.Dramas = mergeHongguoSearchDramas(entry.Dramas, batch)
	}
	reportHongguoSearchProgress(ctx, entry)
	seasonsLimited := downloader.completeHongguoSearchSeasons(ctx, keyword, &entry)
	if errors.Is(ctx.Err(), context.Canceled) {
		return hongguoSearchEntry{}, ctx.Err()
	}
	seasonsLimited = seasonsLimited || errors.Is(ctx.Err(), context.DeadlineExceeded)
	query := keyword
	sort.SliceStable(entry.Dramas, func(left, right int) bool {
		leftTitle, rightTitle := entry.Dramas[left].DisplayTitle(), entry.Dramas[right].DisplayTitle()
		leftRank, rightRank := hongguoTitleSearchRank(leftTitle, query), hongguoTitleSearchRank(rightTitle, query)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		leftBase, leftSeason, leftUnit, _ := hongguoSearchSeason(leftTitle)
		rightBase, rightSeason, rightUnit, _ := hongguoSearchSeason(rightTitle)
		if leftSeason > 0 && rightSeason > 0 && leftUnit == rightUnit && hongguoSearchText(leftBase) == hongguoSearchText(rightBase) {
			return leftSeason < rightSeason
		}
		return false
	})
	if pageErr != nil {
		entry.Warning = "红果综合搜索暂不可用，已保留名称匹配结果，可重试"
	} else if namesErr != nil {
		entry.Warning = "红果名称检索暂不可用，结果可能缺少部分剧集，可重试"
	}
	if seasonsLimited {
		if entry.Warning != "" {
			entry.Warning += "；"
		}
		entry.Warning += "部分季数暂未补齐，已保留当前结果，可重试"
	}
	entry.Limited = entry.Limited || pageErr != nil || namesErr != nil || seasonsLimited
	if entry.Total < len(entry.Dramas) {
		entry.Total = len(entry.Dramas)
	}
	return entry, nil
}

func mergeHongguoSearchDramas(dramas, batch []Drama) []Drama {
	positions := make(map[string]int, len(dramas)+len(batch))
	for index, drama := range dramas {
		positions[drama.ID] = index
	}
	for _, drama := range batch {
		if drama.ID == "" {
			continue
		}
		if index, found := positions[drama.ID]; found {
			dramas[index] = mergeDramaMetadata(dramas[index], drama)
		} else {
			positions[drama.ID] = len(dramas)
			dramas = append(dramas, drama)
		}
	}
	return dramas
}

func (downloader *Downloader) fetchHongguoSearchNames(ctx context.Context, keyword string) ([]Drama, error) {
	records, err := downloader.fetchHongguoSuggestionRecords(ctx, keyword, hongguoSearchNameCount)
	if err != nil {
		return nil, err
	}
	dramas := make([]Drama, 0, len(records))
	for _, record := range records {

		if record.WordType != "short_play_name" || !hongguoNumericID.MatchString(mapString(record.VideoData, "series_id_str", "series_id")) {
			continue
		}
		drama := hongguoDramaFromAny(map[string]any{"video_data": record.VideoData, "name": record.Name, "keyword": record.Keyword}, "短剧")
		if drama.ID != "" && drama.DisplayTitle() != drama.SourceID {
			dramas = append(dramas, drama)
		}
	}
	return dramas, nil
}

func cloneHongguoSearchEntry(entry hongguoSearchEntry) hongguoSearchEntry {
	entry.Dramas = append([]Drama{}, entry.Dramas...)
	for index := range entry.Dramas {
		entry.Dramas[index].Tags = append([]string(nil), entry.Dramas[index].Tags...)
	}
	return entry
}


func hongguoTitleSearchRank(title, query string) int {
	words := strings.FieldsFunc(norm.NFKC.String(query), func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
	query = hongguoSearchText(query)
	title = hongguoSearchText(title)
	switch {
	case title == query:
		return 0
	case strings.HasPrefix(title, query):
		return 1
	case strings.Contains(title, query):
		return 2
	default:
		matches := 0
		for _, word := range words {
			if strings.Contains(title, hongguoSearchText(word)) {
				matches++
			}
		}
		if len(words) > 0 && matches == len(words) {
			return 3
		}
		return 4
	}
}

func (downloader *Downloader) fetchHongguoSearchPage(ctx context.Context, keyword string) (hongguoSearchEntry, error) {
	pageURL := hongguoBaseURL + "/search/" + url.PathEscape(keyword)
	body, err := downloader.fetchProviderText(ctx, pageURL, hongguoBaseURL+"/")
	if err != nil {
		return hongguoSearchEntry{}, err
	}
	page := routerLoaderMap(parseRouterData(body), "search_(keyword)/page", "search_")
	rows, valid := page["searchList"].([]any)
	if page["isSuccess"] != true || !valid || mapString(page, "query") != keyword {
		return hongguoSearchEntry{}, errors.New("红果搜索未返回有效结果")
	}
	entry := hongguoSearchEntry{Dramas: make([]Drama, 0, len(rows))}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		if len(nestedMap(row, "video_data")) == 0 {
			continue
		}
		drama := hongguoDramaFromAny(row, "短剧")
		if drama.ID != "" && !seen[drama.ID] {
			seen[drama.ID] = true
			entry.Dramas = append(entry.Dramas, drama)
		}
	}
	if len(rows) > 0 && len(entry.Dramas) == 0 {
		return hongguoSearchEntry{}, errors.New("红果搜索结果中没有可识别的剧集")
	}
	entry.Total, _ = strconv.Atoi(mapString(page, "totalCount"))

	entry.Limited = entry.Total > len(entry.Dramas)
	if entry.Total < len(entry.Dramas) {
		entry.Total = len(entry.Dramas)
	}
	return entry, nil
}

// --- migrated from provider_hongguo_search_seasons.go ---
const (
	hongguoSearchSeasonLimit   = 200
	hongguoSearchSeasonQueries = 32
	hongguoSearchSeasonTimeout = 25 * time.Second
)

var hongguoSearchSeasonSuffix = regexp.MustCompile(`第\s*([0-9零〇一二两兩三四五六七八九十百]+)\s*([季部])[\p{P}\s]*$`)

type hongguoSearchSeries struct {
	title   string
	key     string
	unit    string
	maximum int
	known   map[int]bool
	labels  map[int]string
}

func hongguoSearchSeason(title string) (string, int, string, string) {
	title = norm.NFKC.String(strings.TrimSpace(title))
	match := hongguoSearchSeasonSuffix.FindStringSubmatchIndex(title)
	if match == nil {
		return "", 0, "", ""
	}
	base := strings.TrimRightFunc(title[:match[0]], func(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) })
	label, unit := title[match[2]:match[3]], title[match[4]:match[5]]
	number, err := strconv.Atoi(label)
	if err != nil {
		number = 0
		digits := map[rune]int{'零': 0, '〇': 0, '一': 1, '二': 2, '两': 2, '兩': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
		if !strings.ContainsAny(label, "十百") {
			for _, r := range label {
				value, ok := digits[r]
				if !ok {
					return "", 0, "", ""
				}
				number = number*10 + value
				if number > hongguoSearchSeasonLimit {
					return "", 0, "", ""
				}
			}
		} else {
			digit, previous := 0, 1000
			for _, r := range label {
				if value, ok := digits[r]; ok {
					digit = digit*10 + value
					continue
				}
				value := 10
				if r == '百' {
					value = 100
				}
				if value >= previous || digit > 9 {
					return "", 0, "", ""
				}
				number += max(digit, 1) * value
				digit, previous = 0, value
			}
			number += digit
		}
	}
	if base == "" || number < 1 || number > hongguoSearchSeasonLimit {
		return "", 0, "", ""
	}
	return base, number, unit, label
}

func hongguoSearchChineseSeason(number int) string {
	digits := []rune("零一二三四五六七八九")
	if number < 10 {
		return string(digits[number])
	}
	if number < 100 {
		prefix := ""
		if number >= 20 {
			prefix = string(digits[number/10])
		}
		prefix += "十"
		if number%10 != 0 {
			prefix += string(digits[number%10])
		}
		return prefix
	}
	prefix, rest := string(digits[number/100])+"百", number%100
	if rest == 0 {
		return prefix
	}
	if rest < 10 {
		prefix += "零"
	} else if rest < 20 {
		prefix += "一"
	}
	return prefix + hongguoSearchChineseSeason(rest)
}

func (series *hongguoSearchSeries) add(drama Drama) bool {
	base, season, unit, label := hongguoSearchSeason(drama.DisplayTitle())
	if season == 0 && hongguoSearchText(drama.DisplayTitle()) == series.key {
		season = 1
	} else if season == 0 || unit != series.unit || hongguoSearchText(base) != series.key {
		return false
	}
	series.known[season] = true
	series.maximum = max(series.maximum, season)
	if label != "" {
		series.labels[season] = label
	}
	return true
}

func hongguoSearchSeriesGroups(keyword string, dramas []Drama) []*hongguoSearchSeries {
	if _, season, _, _ := hongguoSearchSeason(keyword); season > 0 {
		return nil
	}
	query := hongguoSearchText(keyword)
	var groups []*hongguoSearchSeries
	byKey := map[string]*hongguoSearchSeries{}
	for _, drama := range dramas {
		base, season, unit, _ := hongguoSearchSeason(drama.DisplayTitle())
		key := hongguoSearchText(base)
		if season == 0 || !strings.Contains(key, query) {
			continue
		}
		groupKey := key + ":" + unit
		if byKey[groupKey] == nil {
			series := &hongguoSearchSeries{title: base, key: key, unit: unit, known: map[int]bool{}, labels: map[int]string{}}
			byKey[groupKey] = series
			groups = append(groups, series)
		}
	}
	var incomplete []*hongguoSearchSeries
	for _, group := range groups {
		for _, drama := range dramas {
			group.add(drama)
		}
		if len(group.known) >= 2 && len(group.known) < group.maximum {
			incomplete = append(incomplete, group)
		}
	}
	sort.SliceStable(incomplete, func(left, right int) bool {
		l, r := incomplete[left], incomplete[right]
		lr, rr := hongguoTitleSearchRank(l.title, query), hongguoTitleSearchRank(r.title, query)
		if lr != rr {
			return lr < rr
		}
		return len(l.known) > len(r.known)
	})
	return incomplete
}

func (series *hongguoSearchSeries) nextQuery(attempted map[string]bool) string {
	for season := 1; season <= series.maximum; season++ {
		if series.known[season] {
			continue
		}
		nearest, distance := 0, hongguoSearchSeasonLimit+1
		for known := 1; known <= series.maximum; known++ {
			delta := max(known-season, season-known)
			if series.labels[known] != "" && delta < distance {
				nearest, distance = known, delta
			}
		}
		labels := []string{hongguoSearchChineseSeason(season), strconv.Itoa(season)}
		if _, err := strconv.Atoi(series.labels[nearest]); err == nil {
			labels[0], labels[1] = labels[1], labels[0]
		}
		var queries []string
		if season == 1 {
			queries = append(queries, series.title)
		}
		for _, label := range labels {
			queries = append(queries, series.title+"第"+label+series.unit)
		}
		for _, query := range queries {
			if _, err := hongguoSearchKeyword(query); err == nil && !attempted[query] {
				return query
			}
		}
	}
	return ""
}

func (downloader *Downloader) completeHongguoSearchSeasons(ctx context.Context, keyword string, entry *hongguoSearchEntry) bool {
	groups := hongguoSearchSeriesGroups(keyword, entry.Dramas)
	if len(groups) == 0 {
		return false
	}
	budget := hongguoSearchSeasonTimeout
	if deadline, ok := ctx.Deadline(); ok {
		budget = min(budget, time.Until(deadline)-100*time.Millisecond)
	}
	if budget <= 0 {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	attempted := map[string]bool{keyword: true}
	requests := 0
	for _, group := range groups {
		for requests < hongguoSearchSeasonQueries {
			query := group.nextQuery(attempted)
			if query == "" {
				break
			}
			attempted[query] = true
			requests++
			names, err := downloader.fetchHongguoSearchNames(ctx, query)
			if err != nil {
				return true
			}
			var matches []Drama
			for _, drama := range names {
				if group.add(drama) {
					matches = append(matches, drama)
				}
			}
			if len(matches) > 0 {
				entry.Dramas = mergeHongguoSearchDramas(entry.Dramas, matches)
				reportHongguoSearchProgress(ctx, *entry)
			}
		}
	}
	for _, group := range groups {
		if len(group.known) < group.maximum {
			return true
		}
	}
	return false
}

// --- migrated from provider_hongguo_comment_sign.go ---
type hongguoCommentNonce [11]byte

func newHongguoCommentNonce() (hongguoCommentNonce, error) {
	var nonce hongguoCommentNonce
	_, err := rand.Read(nonce[:])
	return nonce, err
}

func signHongguoCommentRequest(request *http.Request, nonce hongguoCommentNonce, now time.Time) {
	stamp := uint32(now.Unix())
	query := request.URL.RawQuery
	head2, head3 := (nonce[0]%7+1)<<5, 0xe0|nonce[1]&15
	seed := make([]byte, 20)
	hash := md5.Sum([]byte(query))
	copy(seed, hash[:4])
	copy(seed[12:16], []byte{0, 1, 7, 4})
	binary.BigEndian.PutUint32(seed[16:], stamp)
	key := [8]byte{0x4a, 0, 0x16, head3, 0x47, 0x6c, 0, head2}
	var box [256]byte
	for i := range box {
		box[i] = byte(i)
	}
	j := byte(0)
	for i := range box {
		j += box[i] + key[i&7]
		box[i] = box[j]
	}
	j = 0
	for offset := range seed {
		i := byte(offset + 1)
		j += box[i]
		box[i] = box[j]
		seed[offset] ^= box[box[i]+box[j]]
	}
	for i := range seed {
		seed[i] = bits.Reverse8(bits.RotateLeft8(seed[i], 4)^seed[(i+1)%len(seed)]) ^ 0xeb
	}
	request.Header.Set("X-Gorgon", hex.EncodeToString(append([]byte{0x84, 4, head2, head3, 0, 0}, seed...)))
	request.Header.Set("X-Khronos", strconv.FormatUint(uint64(stamp), 10))
	request.Header.Set("X-SS-Req-Ticket", strconv.FormatInt(now.UnixMilli(), 10))
	request.Header.Set("X-SS-STUB", "")
	request.Header.Set("X-Argus", hongguoCommentArgus(query, stamp, nonce))
	request.Header.Set("X-Ladon", hongguoCommentLadon(request.URL.Query().Get("aid"), stamp, nonce[2:6]))
}

func hongguoCommentPad(data []byte) []byte {
	padding := 16 - len(data)%16
	out := append([]byte(nil), data...)
	for i := 0; i < padding; i++ {
		out = append(out, byte(padding))
	}
	return out
}

func hongguoCommentLadon(aid string, timestamp uint32, prefix []byte) string {
	hash := md5.Sum(append(append([]byte(nil), prefix...), aid...))
	digest := []byte(hex.EncodeToString(hash[:]))
	left, right := binary.LittleEndian.Uint64(digest), binary.LittleEndian.Uint64(digest[8:])
	queue := []uint64{binary.LittleEndian.Uint64(digest[16:]), binary.LittleEndian.Uint64(digest[24:])}
	var keys [34]uint64
	for i := range keys {
		keys[i] = left
		mixed := (bits.RotateLeft64(right, -8) + left) ^ uint64(i)
		queue = append(queue, mixed)
		left = mixed ^ bits.RotateLeft64(left, 3)
		right, queue = queue[0], queue[1:]
	}
	data := hongguoCommentPad([]byte(strconv.FormatUint(uint64(timestamp), 10) + "-1611921764-3019"))
	for offset := 0; offset < len(data); offset += 16 {
		left, right := binary.LittleEndian.Uint64(data[offset:]), binary.LittleEndian.Uint64(data[offset+8:])
		for _, key := range keys {
			right = key ^ (left + bits.RotateLeft64(right, -8))
			left = right ^ bits.RotateLeft64(left, 3)
		}
		binary.LittleEndian.PutUint64(data[offset:], left)
		binary.LittleEndian.PutUint64(data[offset+8:], right)
	}
	return base64.StdEncoding.EncodeToString(append(append([]byte(nil), prefix...), data...))
}

func hongguoCommentArgus(query string, timestamp uint32, nonce hongguoCommentNonce) string {
	key, _ := hex.DecodeString("ac1adaae95a7af94a5114ab3b3a97dd80050aa0a39314c40528caec95256c28c")
	var message []byte
	varint := func(field, value uint64) {
		message = binary.AppendUvarint(message, field*8)
		message = binary.AppendUvarint(message, value)
	}
	bytesField := func(field uint64, value []byte) {
		message = binary.AppendUvarint(message, field*8+2)
		message = binary.AppendUvarint(message, uint64(len(value)))
		message = append(message, value...)
	}
	varint(1, 0x20200929*2)
	varint(2, 2)
	varint(3, uint64(binary.LittleEndian.Uint32(nonce[6:10])))
	bytesField(4, []byte("3019"))
	bytesField(6, []byte("1611921764"))
	bytesField(7, []byte("6.8.1.32"))
	bytesField(8, []byte("v04.07.01-ml-android"))
	varint(9, 135135744)
	bytesField(10, make([]byte, 8))
	varint(12, uint64(timestamp)*2)
	bodyHash, queryHash := hongguoSM3(make([]byte, 16)), hongguoSM3([]byte(query))
	bytesField(13, bodyHash[:6])
	bytesField(14, queryHash[:6])
	nested := []byte{8, 2 + (nonce[10]%49)*2}
	for _, field := range []uint64{2, 3, 4} {
		nested = binary.AppendUvarint(nested, field*8)
		nested = binary.AppendUvarint(nested, 1388734)
	}
	bytesField(15, nested)
	bytesField(20, []byte("none"))
	varint(21, 738)
	data := hongguoCommentPad(message)
	seed := append(append(append([]byte(nil), key...), 0x3c, 0xcc, 0x56, 0x7b), key...)
	derived := hongguoSM3(seed)
	var rounds [72]uint64
	for i := 0; i < 4; i++ {
		rounds[i] = binary.LittleEndian.Uint64(derived[i*8:])
	}
	for i := 4; i < len(rounds); i++ {
		mixed := bits.RotateLeft64(rounds[i-1], -3) ^ rounds[i-3]
		mixed ^= bits.RotateLeft64(mixed, -1)
		rounds[i] = ^rounds[i-4] ^ mixed ^ (uint64(0x3dc94c3a046d678b)>>uint((i-4)%62))&1 ^ 3
	}
	for offset := 0; offset < len(data); offset += 16 {
		left, right := binary.LittleEndian.Uint64(data[offset:]), binary.LittleEndian.Uint64(data[offset+8:])
		for _, round := range rounds {
			left, right = right, left^(bits.RotateLeft64(right, 1)&bits.RotateLeft64(right, 8))^bits.RotateLeft64(right, 2)^round
		}
		binary.LittleEndian.PutUint64(data[offset:], left)
		binary.LittleEndian.PutUint64(data[offset+8:], right)
	}
	xor := []byte{0xd0, 0x4f, 0xfd, 0xff, 0xd0, 0x4f, 0xfd, 0xff}
	for i := range data {
		data[i] ^= xor[i%8]
	}
	data = append(xor, data...)
	for left, right := 0, len(data)-1; left < right; left, right = left+1, right-1 {
		data[left], data[right] = data[right], data[left]
	}
	container := append([]byte{0xa6, 0xe7, 0x83, 0xee, 0x70, 0x01, 0x10, 0x09, 0x18}, data...)
	container = hongguoCommentPad(append(container, 0x56, 0x7b))
	aesKey, iv := md5.Sum(key[:16]), md5.Sum(key[16:])
	block, _ := aes.NewCipher(aesKey[:])
	cipher.NewCBCEncrypter(block, iv[:]).CryptBlocks(container, container)
	return base64.StdEncoding.EncodeToString(append([]byte{0x3c, 0xcc}, container...))
}

func hongguoSM3(input []byte) [32]byte {
	length := uint64(len(input)) * 8
	data := append(append([]byte(nil), input...), 0x80)
	for len(data)%64 != 56 {
		data = append(data, 0)
	}
	data = binary.BigEndian.AppendUint64(data, length)
	state := [8]uint32{0x7380166f, 0x4914b2b9, 0x172442d7, 0xda8a0600, 0xa96f30bc, 0x163138aa, 0xe38dee4d, 0xb0fb0e4e}
	for offset := 0; offset < len(data); offset += 64 {
		var words [68]uint32
		for i := 0; i < 16; i++ {
			words[i] = binary.BigEndian.Uint32(data[offset+i*4:])
		}
		for i := 16; i < len(words); i++ {
			v := words[i-16] ^ words[i-9] ^ bits.RotateLeft32(words[i-3], 15)
			words[i] = v ^ bits.RotateLeft32(v, 15) ^ bits.RotateLeft32(v, 23) ^ bits.RotateLeft32(words[i-13], 7) ^ words[i-6]
		}
		a, b, c, d, e, f, g, h := state[0], state[1], state[2], state[3], state[4], state[5], state[6], state[7]
		for i := 0; i < 64; i++ {
			t, ff, gg := uint32(0x79cc4519), a^b^c, e^f^g
			if i >= 16 {
				t, ff, gg = 0x7a879d8a, (a&b)|(a&c)|(b&c), (e&f)|(^e&g)
			}
			ss1 := bits.RotateLeft32(bits.RotateLeft32(a, 12)+e+bits.RotateLeft32(t, i), 7)
			tt1 := ff + d + (ss1 ^ bits.RotateLeft32(a, 12)) + (words[i] ^ words[i+4])
			tt2 := gg + h + ss1 + words[i]
			a, b, c, d = tt1, a, bits.RotateLeft32(b, 9), c
			e, f, g, h = tt2^bits.RotateLeft32(tt2, 9)^bits.RotateLeft32(tt2, 17), e, bits.RotateLeft32(f, 19), g
		}
		for i, value := range [8]uint32{a, b, c, d, e, f, g, h} {
			state[i] ^= value
		}
	}
	var result [32]byte
	for i, value := range state {
		binary.BigEndian.PutUint32(result[i*4:], value)
	}
	return result
}

func parseHongguoAppDetail(result map[string]any, seriesID string) (hongguoDetailEntry, error) {
	detail := nestedMap(result, "data", "video_data")
	returnedID := mapString(detail, "series_id_str", "series_id")
	if returnedID != seriesID {
		return hongguoDetailEntry{}, errors.New("红果 App 未返回所请求的剧集")
	}
	entry := hongguoDetailEntry{Drama: hongguoDramaFromAny(detail, "短剧")}
	seen := map[string]bool{}
	episodes := map[int]bool{}
	for _, row := range anyList(detail["video_list"]) {
		video, _ := row.(map[string]any)
		videoID := mapString(video, "vid")
		index, err := strconv.Atoi(mapString(video, "vid_index"))
		if err != nil || index < 1 || !hongguoNumericID.MatchString(videoID) {
			return entry, errors.New("红果 App 分集编号或视频 ID 无效")
		}
		if identifier := mapString(video, "series_id"); identifier != "" && identifier != seriesID {
			return entry, errors.New("红果 App 返回了其他剧集的分集")
		}
		if seen[videoID] || episodes[index] {
			return entry, errors.New("红果 App 返回了重复分集")
		}
		seen[videoID], episodes[index] = true, true
		entry.Chapters = append(entry.Chapters, Chapter{
			ID: providerChapterID(sourceHongguo, seriesID, videoID), Source: sourceHongguo,
			Title: fmt.Sprintf("第%d集", index), VideoURL: "hongguo-cenc://" + videoID, CurrentEpisode: rawEpisode(index),
		})
	}
	sort.Slice(entry.Chapters, func(left, right int) bool {
		leftIndex, _ := strconv.Atoi(entry.Chapters[left].EpisodeString(left + 1))
		rightIndex, _ := strconv.Atoi(entry.Chapters[right].EpisodeString(right + 1))
		return leftIndex < rightIndex
	})
	total, _ := strconv.Atoi(mapString(detail, "episode_cnt"))
	if len(entry.Chapters) == 0 || total > len(entry.Chapters) {
		return entry, errors.New("红果 App 未返回完整分集，已尝试其他详情入口")
	}
	for index, chapter := range entry.Chapters {
		if chapter.EpisodeString(index+1) != strconv.Itoa(index+1) {
			return entry, errors.New("红果 App 分集列表不连续")
		}
	}
	return entry, nil
}

func selectHongguoAppMedia(model map[string]any) (providerMedia, error) {
	variants := anyList(model["video_list"])
	if rows, ok := model["video_list"].(map[string]any); ok && len(variants) == 0 {
		keys := make([]string, 0, len(rows))
		for key := range rows {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			variants = append(variants, rows[key])
		}
	}
	duration, _ := strconv.ParseFloat(mapString(model, "video_duration", "duration"), 64)
	var keyErr error
	type scoredMedia struct {
		media providerMedia
		score int
	}
	var choices []scoredMedia
	for _, row := range variants {
		variant, _ := row.(map[string]any)
		meta := nestedMap(variant, "video_meta")
		codec := strings.ToLower(mapString(meta, "codec_type"))
		if codec == "bytevc2" || strings.Contains(strings.ToLower(mapString(variant, "gear_des_key")), "bytevc2") {
			continue
		}
		addresses := hongguoMediaAddresses(variant)
		if len(addresses) == 0 {
			continue
		}
		media := providerMedia{Referer: "https://novel.snssdk.com/", Duration: time.Duration(duration * float64(time.Second))}
		encryption := nestedMap(variant, "encrypt_info")
		spade := mapString(encryption, "spade_a")
		if spade != "" || encryption["encrypt"] == true || mapString(encryption, "encryption_method") == "cenc-aes-ctr" {
			var err error
			media.CENCKey, err = hongguoContentKey(spade)
			if err != nil {
				keyErr = err
				continue
			}
		}
		height, _ := strconv.Atoi(mapString(meta, "vheight"))
		if definition, err := strconv.Atoi(hongguoQualityNumber.FindString(mapString(meta, "definition"))); err == nil && definition > 0 {
			height = definition
		} else if width, _ := strconv.Atoi(mapString(meta, "vwidth")); width > 0 && (height == 0 || width < height) {
			height = width
		}
		media.Quality = height
		quality := height * 10
		if codec == "h264" || codec == "avc1" {
			quality++
		}
		for _, address := range addresses {
			media.URL = address
			choices = append(choices, scoredMedia{media: media, score: quality})
		}
	}
	if len(choices) > 0 {
		sort.SliceStable(choices, func(i, j int) bool { return choices[i].score > choices[j].score })
		selected := choices[0].media
		for _, choice := range choices {
			selected.Variants = append(selected.Variants, choice.media)
		}
		return selected, nil
	}
	if keyErr != nil {
		return providerMedia{}, fmt.Errorf("红果 App 媒体密钥不可用: %w", keyErr)
	}
	return providerMedia{}, errors.New("红果 App 未返回兼容的媒体，已跳过不支持的编码")
}

func hongguoMediaAddresses(info map[string]any) []string {
	var addresses []string
	seen := map[string]bool{}
	var add func(any)
	add = func(value any) {
		switch value := value.(type) {
		case string:
			address := strings.TrimSpace(value)
			if len(address) > 8192 {
				return
			}
			if !isProviderHTTPMediaURL(address) {
				decoded, err := decodeHongguoBase64(address)
				if err != nil {
					return
				}
				address = strings.TrimSpace(string(decoded))
			}
			if isProviderHTTPMediaURL(address) && !seen[address] {
				seen[address] = true
				addresses = append(addresses, address)
			}
		case []any:
			for _, item := range value {
				add(item)
			}
		}
	}
	for _, key := range []string{"main_url", "backup_url", "backup_url_1", "backup_url_2", "backup_urls", "url_list"} {
		add(info[key])
	}
	return addresses
}

func hongguoContentKey(value string) ([]byte, error) {
	if len(value) > 1024 {
		return nil, errors.New("红果媒体密钥数据过长")
	}
	raw, err := decodeHongguoBase64(value)
	if err != nil || len(raw) < 3 {
		return nil, errors.New("红果媒体密钥编码无效")
	}
	tagLength := int(raw[0]^raw[1]^raw[2]) - 48
	contentLength := len(raw) - tagLength - 1
	if tagLength < 1 || contentLength < 33 || contentLength >= len(raw) {
		return nil, errors.New("红果媒体密钥结构无效")
	}
	seed := raw[len(raw)-tagLength-2] ^ raw[len(raw)-tagLength-1]
	tag := make([]byte, tagLength)
	for index := range tag {
		tag[index] = raw[len(raw)-tagLength+index] ^ seed
	}
	if string(tag) == "app_v2" || string(tag) == "web_v2" {
		return nil, errors.New("红果媒体密钥版本暂不支持")
	}
	decoded := make([]byte, contentLength)
	previousEven, previousOdd := byte(250), byte(85)
	for index, current := range raw[1 : 1+contentLength] {
		previous := previousEven
		if index%2 == 0 {
			previousEven = current
		} else {
			previous = previousOdd
			previousOdd = current
		}
		decoded[index] = byte(int(previous^current) - 21 - bits.OnesCount(uint(index)))
	}
	padding, err := strconv.ParseUint(string(decoded[:1]), 36, 8)
	if err != nil || contentLength-int(padding)-1 != 32 {
		return nil, errors.New("红果媒体密钥内容无效")
	}
	key, err := hex.DecodeString(string(decoded[1:33]))
	if err != nil || len(key) != aes.BlockSize {
		return nil, fmt.Errorf("红果媒体密钥不是有效的 AES-128 密钥")
	}
	return key, nil
}

var hongguoQualityNumber = regexp.MustCompile(`[0-9]+`)

func decodeHongguoBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.Strict().DecodeString(value)
}
