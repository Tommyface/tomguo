package core

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	dataDir             string
	MaxPagesPerSort     int
	PageSize            int
	Retries             int
	InsecureTLS         bool
	HuangguoAIURL       string
	HuangguoWorkerURL   string
	HuangdouPlatformKey string
	HuangguoVideoURL    string
	HuangdouURL         string
	HongguoURL          string
	HuangjuURL          string
	HuangjuAPIURL       string
	YeguoURL            string
	YeguoAPIURL         string
	DSDURL              string
	SoraniURL           string
	SoraniAPIURL        string
	GuipianURL          string
	HanxiaoquanURL      string
	ChengguoURL         string
	ChengguoAPIURL      string
	Token               string
	AESKeyHex           string
	InterfaceKey        string
	ParamKey            string
	ParamIV             string
	APIBase             string
	CDNURL              string
}

type Downloader struct {
	rankings              rankingCache
	cfg                   Config
	client                *http.Client
	huangdouDetails       map[string]huangdouDetailEntry
	huangdouDetailPending map[string]*huangdouDetailCall
	providerMu            sync.Mutex
	providerHosts         map[string]string
	limiter               *requestLimiter
	proxyRouter           *proxyRouter
	hongguoOnce           sync.Once
	hongguo               *hongguoAppClient
	yeguoOnce             sync.Once
	yeguo                 *yeguoAPIClient
	dsdCatalog            dsdCatalogState
	diagnostics           *diagnosticLog
	apiMu                 sync.Mutex
	apiBase               string
	apiFailures           map[string]time.Time
	legacyOnce            sync.Once
	legacy                *legacyAPIClient
	previewMu             sync.Mutex
	previewSessions       map[string]*huangguoPreviewSession
	customSources         *customRegistry
}

func defaultConfig() Config { return Config{MaxPagesPerSort: 50, PageSize: 30, Retries: 2} }

type nativeDrama struct {
	MetadataSchema int      `json:"metadataSchema"`
	ID             string   `json:"id"`
	Source         string   `json:"source"`
	SourceID       string   `json:"sourceId"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	Cover          string   `json:"cover"`
	Episodes       int      `json:"episodes"`
	Category       string   `json:"category"`
	VIP            *bool    `json:"vip"`
	Heat           string   `json:"heat,omitempty"`
	Views          string   `json:"views,omitempty"`
	OnlineDate     string   `json:"onlineDate,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	ReleaseStatus  string   `json:"releaseStatus,omitempty"`
}

type nativeInput struct {
	LAN              json.RawMessage         `json:"lan"`
	ExpectedVersions map[string]string       `json:"expectedVersions"`
	SystemProxy      nativeSystemProxy       `json:"systemProxy"`
	Settings         nativeResourceSettings  `json:"settings"`
	JobIDs           []string                `json:"jobIds"`
	PlaybackSession  string                  `json:"playbackSession"`
	StartMS          int64                   `json:"startMs"`
	DurationMS       int64                   `json:"durationMs"`
	Board            string                  `json:"board"`
	Entries          []nativeDownloadEpisode `json:"entries"`
	JobID            string                  `json:"jobId"`
	Command          string                  `json:"command"`
	Action           string                  `json:"action"`
	Directory        string                  `json:"directory"`
	Source           string                  `json:"source"`
	Page             int                     `json:"page"`
	Query            string                  `json:"query"`
	Category         string                  `json:"category"`
	Drama            nativeDrama             `json:"drama"`
	Chapter          Chapter                 `json:"chapter"`
	Index            int                     `json:"index"`
	Quality          int                     `json:"quality"`
	Session          string                  `json:"session"`
	Sequence         int64                   `json:"sequence"`
	Force            bool                    `json:"force"`
	CustomSources    json.RawMessage         `json:"customSources"`
	ModulePath       string                  `json:"modulePath"`
	RegistryURL      string                  `json:"registryUrl"`
	IncludeAdult     bool                    `json:"includeAdult"`
	Sources          []string                `json:"sources"`
	AdultView        bool                    `json:"adultView"`
	ContentKind      string                  `json:"contentKind"`
	AggregateKey     string                  `json:"aggregateKey"`
	GroupID          string                  `json:"groupId"`
	Refine           bool                    `json:"refine"`
	// Mode 决定聚合的取数策略：
	//   instant  只读内存/磁盘缓存，绝不联网（首屏用）
	//   refresh  缓存优先，缺失或过期时才联网
	//   force    强制联网（用户明确要求）
	// 空值等价于 refresh，保持旧调用的行为。
	Mode string `json:"mode"`
	// Stage 是门面动作内部的子阶段，用于把多个旧 action 收进一个入口：
	//   stream: resolve | fallback
	//   aggregate: page | sources
	Stage         string `json:"stage"`
	SourcesTagged bool   `json:"sourcesTagged"`
	CachedOnly    bool   `json:"cachedOnly"`
}

type nativeCatalogResult struct {
	Items       []nativeDrama `json:"items"`
	HasMore     bool          `json:"hasMore"`
	Page        int           `json:"page"`
	Warning     string        `json:"warning,omitempty"`
	LocalSearch bool          `json:"localSearch"`
	Fresh       bool          `json:"fresh"`
	hongguo     *hongguoCatalogState
	saveError   error
}

type nativePlan struct {
	ExpiresAt       int64             `json:"expiresAt,omitempty"`
	PrefetchedBytes int64             `json:"prefetchedBytes,omitempty"`
	DanmakuID       string            `json:"danmakuId,omitempty"`
	Local           bool              `json:"local"`
	URL             string            `json:"url"`
	Headers         map[string]string `json:"headers"`
	Key             string            `json:"decryptionKey,omitempty"`
	Quality         int               `json:"quality"`
	Qualities       []int             `json:"qualities"`
	Session         string            `json:"session,omitempty"`
	RouteIndex      int               `json:"routeIndex"`
	RouteCount      int               `json:"routeCount"`
}

type nativeEngine struct {
	lanMu            sync.Mutex
	lan              *nativeLANServer
	settingsMu       sync.Mutex
	settings         nativeResourceSettings
	readMu           sync.Mutex
	reads            map[string]nativeReadRequest
	readCount        int
	work             map[string]bool
	downloads        *nativeDownloads
	downloader       *Downloader
	directory        string
	mu               sync.Mutex
	catalogs         map[string][]nativeDrama
	catalogStates    map[string]nativeCatalogState
	sourceProbes     map[string]nativeSourceProbe
	coverBackfill    map[string]time.Time
	categoryOptions  map[string][]nativeCategory
	hongguoCatalog   *hongguoCatalogState
	recommendations  map[string]nativeRecommendationState
	catalogSaveError error
	// catalogSaveTimer 是剧库写盘的防抖定时器：一轮刷新里每个站源抓完
	// 都会碰一次剧库，原来每次都全量 Marshal + 写盘 + fsync，七个源就是
	// 七次重 IO，而且都在锁内，把首页的其它 IPC 一起堵住。
	catalogSaveTimer *time.Timer
	sourceSaveError  error
	covers           *nativeCoverCache
	stream           *nativeStreamServer
	playbacks        map[string]nativePlaybackChoice
	playbackMu       sync.Mutex
	playbackSequence int64
	playbackCancel   context.CancelFunc
	sourceTasks      map[string]*nativeSourceTask
	sourceRecords    map[string]nativeSourceRecord
	sourceCatalogMu  sync.Mutex
	sourceCatalogs   map[string]chan struct{}
}

var nativeState struct {
	sync.Mutex
	engine *nativeEngine
}

func nativeText(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		for _, key := range []string{"url", "src", "cover", "image", "pic"} {
			if text := nativeText(v[key]); text != "" {
				return text
			}
		}
	case []any:
		for _, entry := range v {
			if text := nativeText(entry); text != "" {
				return text
			}
		}
	case json.Number:
		return string(v)
	case float64:
		return strconv.Itoa(int(v))
	case int:
		return strconv.Itoa(v)
	}
	return ""
}

// nativeEpisodeCount 从任意值里提取集数（参考 zhenguojianTV v0.8.31）。
// 新源单测依赖它；同时供移植过来的 provider 代码使用，保持行为一致。
func nativeEpisodeCount(value any) int {
	text := strings.TrimSpace(nativeText(value))
	if text == "" {
		return 0
	}
	if count, err := strconv.Atoi(text); err == nil {
		return max(count, 0)
	}
	start := -1
	for index := 0; index < len(text); index++ {
		digit := text[index] >= '0' && text[index] <= '9'
		if digit {
			if start < 0 {
				start = index
			}
			continue
		}
		if start >= 0 {
			if count, err := strconv.Atoi(text[start:index]); err == nil && count > 0 {
				return count
			}
			start = -1
		}
	}
	if start >= 0 {
		if count, err := strconv.Atoi(text[start:]); err == nil && count > 0 {
			return count
		}
	}
	return 0
}

func nativeNormalize(drama Drama) nativeDrama {
	cover := ""
	for _, value := range []any{drama.Cover, drama.CoverURL, drama.CoverURLSnake, drama.Image, drama.ImageURL, drama.ImageURLSnake, drama.Img, drama.Pic, drama.Picture, drama.Poster, drama.Thumb, drama.Thumbnail} {
		candidate := nativeText(value)
		if strings.HasPrefix(candidate, "//") {
			candidate = "https:" + candidate
		}
		if isProviderHTTPMediaURL(candidate) {
			cover = candidate
			break
		}
	}
	episodes := 0
	for _, value := range []any{drama.TotalEpisode, drama.TotalEpisodeSnake, drama.EpisodeCount, drama.EpisodeCountSnake, drama.ChapterCount, drama.ChapterCountSnake, drama.Total, drama.Episodes} {
		if count, _ := strconv.Atoi(nativeText(value)); count > episodes {
			episodes = count
		}
	}
	source, sourceID, _ := splitProviderDramaID(drama.ID)
	return nativeDrama{MetadataSchema: 1, ID: drama.ID, Source: source, SourceID: sourceID, Title: drama.DisplayTitle(),
		Description: firstNonEmpty(drama.Desc, drama.Intro), Cover: cover, Episodes: episodes,
		Category: nativeDramaCategory(drama), VIP: drama.VIP,
		Heat: drama.Heat, Views: drama.Views, OnlineDate: providerReleaseDate(drama.OnlineDate),
		Tags: append([]string(nil), drama.Tags...), ReleaseStatus: drama.ReleaseStatus}
}

func nativeDramaCategory(drama Drama) string {
	category := firstNonEmpty(drama.CategoryName, drama.CategoryNameSnake, drama.TypeName, drama.TypeNameSnake, drama.SortName, drama.SortNameSnake, drama.Category)
	if category != "" {
		return category
	}
	switch drama.ChannelName {
	case "真人剧", "短剧", "漫剧", "AI 剧", "AI剧", "AI 短剧", "AI 漫剧", "AI 换脸", "AI 魔改":
		return drama.ChannelName
	}
	return ""
}

func newNativeEngine(directory string) (*nativeEngine, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("应用数据目录无效")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	cfg.dataDir = directory
	router := &proxyRouter{}
	d := &Downloader{cfg: cfg, providerHosts: map[string]string{}, proxyRouter: router,
		limiter: newRequestLimiter(3, 250*time.Millisecond), diagnostics: newDiagnosticLog(directory),
		customSources: newCustomRegistry()}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	transport.MaxIdleConnsPerHost = 8
	transport.ResponseHeaderTimeout = 20 * time.Second
	transport.Proxy = router.proxy
	cdn := newCDNTransport(transport, newDNSResolver(transport))
	d.client = &http.Client{Transport: newHuangguoBrowserTransport(cdn, d), Timeout: 45 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("站源重定向次数过多")
			}
			if len(via) > 0 && !strings.EqualFold(request.URL.Host, via[0].URL.Host) {
				request.Header.Del("X-Preview-Token")
			}
			return nil
		}}
	setActiveDownloader(d)
	if _, err := LoadModules(directory); err != nil {
		d.recordDiagnostic(diagnosticEvent{Level: "warn", Event: "modules", Message: err.Error()})
	}
	engine := &nativeEngine{downloader: d, directory: directory, catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}, sourceProbes: map[string]nativeSourceProbe{}}
	engine.loadResourceSettings()
	d.loadRankingCache()
	engine.loadCatalogCache()
	engine.loadSourceRecords()
	engine.covers = newNativeCoverCache(directory, d)
	engine.downloads = newNativeDownloads(engine)
	return engine, nil
}

func NativeRequest(raw string) (result string) {
	defer func() {
		if recover() != nil {
			result = `{"ok":false,"error":"本地核心处理失败，请重试"}`
		}
	}()
	var input nativeInput
	if len(raw) > 1<<20 || json.Unmarshal([]byte(raw), &input) != nil {
		return `{"ok":false,"error":"请求格式无效"}`
	}
	data, err := nativeDispatch(input)
	envelope := map[string]any{"ok": err == nil}
	if err != nil {
		envelope["error"] = publicError(err).Error()
		if errors.Is(err, errNativeLocalFile) {
			envelope["code"] = "local_media"
		}
		var backoff *requestBackoff
		if errors.As(err, &backoff) {
			envelope["code"] = "source_blocked"
			envelope["retryAt"] = backoff.until
		}
		source := canonicalProviderSource(input.Source)
		if source == "" {
			source = sourceFromDramaID(input.Drama.ID)
		}
		if !errors.Is(err, context.Canceled) && (input.Action == "catalog" || input.Action == "detail" || input.Action == "resolve") {
			nativeState.Lock()
			engine := nativeState.engine
			nativeState.Unlock()
			if engine != nil && nativeSourceAvailable(source) {
				engine.changeSourceRecord(source, func(record *nativeSourceRecord) {
					record.Error = publicError(err).Error()
					if backoff != nil {
						record.RetryAt = backoff.until
					}
					if !record.Running {
						record.Stage, record.FinishedAt = "访问失败", time.Now()
					}
				})
			}
		}
	} else {
		envelope["data"] = data
	}
	body, marshalErr := json.Marshal(envelope)
	if marshalErr != nil {
		return `{"ok":false,"error":"无法读取站源返回的数据"}`
	}
	return string(body)
}

func nativeDispatch(input nativeInput) (any, error) {
	if err := nativeAuthorizeInput(input); err != nil {
		return nil, err
	}
	nativeState.Lock()
	if input.Action == "initialize" {
		if nativeState.engine == nil {
			engine, err := newNativeEngine(input.Directory)
			if err != nil {
				nativeState.Unlock()
				return nil, err
			}
			nativeState.engine = engine
		}
		nativeState.Unlock()
		return map[string]any{"version": "0.2.17", "standalone": true, "allSources": true}, nil
	}
	engine := nativeState.engine
	nativeState.Unlock()
	if engine == nil {
		return nil, errors.New("应用核心尚未就绪，请重新打开应用")
	}
	duration := 60 * time.Second
	if input.Action == "moveDownloads" {
		duration = 10 * time.Minute
	} else if input.Action == "danmaku" {
		duration = 10 * time.Second
	} else if input.Action == "preload" {
		duration = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()
	if input.Action == "danmaku" || input.Action == "preload" || input.Action == "prepareHandoff" || input.Session != "" && (input.Action == "catalog" || input.Action == "categories" || input.Action == "suggestions" || input.Action == "recommendations" || input.Action == "metadata") {
		work, finish, err := engine.beginRead(ctx, input)
		if err != nil {
			return nil, err
		}
		defer finish()
		ctx = work
	}
	switch input.Action {
	case "lan":
		return engine.nativeLAN(ctx, input.Command, input.LAN)
	case "registerCustomSources":
		configs, err := decodeCustomSources(input.CustomSources)
		if err != nil {
			return nil, fmt.Errorf("站源配置解析失败: %w", err)
		}
		count, err := engine.downloader.customSources.replace(configs)
		if err != nil {
			return nil, err
		}
		return map[string]any{"count": count, "sources": engine.downloader.customSources.list()}, nil
	case "listCustomSources":
		return map[string]any{"sources": engine.downloader.customSources.list()}, nil
	case "updateSystemProxy":
		return true, engine.updateSystemProxy(input.SystemProxy)
	case "resourceSettings":
		return engine.resourceSettings(), nil
	case "saveResourceSettings":
		return engine.saveResourceSettings(input.Settings)
	case "controlDownloadBatch":
		return engine.downloads.controlBatchExpected(ctx, input.JobIDs, input.Command, input.ExpectedVersions)
	case "preload":
		return engine.nativePreload(ctx, input)
	case "prepareHandoff":
		return engine.nativeResolve(ctx, input)
	case "danmaku":
		return engine.nativeDanmaku(ctx, input)
	case "cancelRead":
		engine.cancelRead(input)
		return true, nil
	case "recommendations":
		return engine.nativeRecommendations(ctx, input)
	case "cachedRecommendations":
		return engine.cachedRecommendations(input.Category)
	case "rankingBoards":
		boards := []rankingBoard{}
		for _, board := range rankingBoards {
			// 退役内置源与无榜单实现能力的源都不下发，避免点了没内容。
			if isRetiredSource(board.Source) || !nativeSourceAvailable(board.Source) ||
				!nativeRankingBoardSupported(board) {
				continue
			}
			boards = append(boards, board)
		}
		return map[string]any{"items": boards}, nil
	case "rankings":
		return engine.nativeRanking(ctx, input)
	case "suggestions":
		items, err := engine.suggestions(ctx, input.Query)
		return map[string]any{"items": items}, err
	case "downloadDirectory":
		engine.downloads.mu.Lock()
		root := engine.downloads.root
		engine.downloads.mu.Unlock()
		return map[string]string{"directory": root}, nil
	case "storage":
		return engine.storage()
	case "moveDownloads":
		return true, engine.moveDownloads(ctx, input.Directory)
	case "workLease":
		count, err := engine.workLease(input.JobID, input.Command)
		return map[string]int{"count": count}, err
	case "downloads":
		jobs, err := engine.downloads.snapshot()
		return map[string]any{"jobs": jobs}, err
	case "enqueueDownloads":
		added, err := engine.downloads.enqueueContext(ctx, input)
		return map[string]int{"added": added}, err
	case "controlDownloads":
		return true, engine.downloads.control(input.JobID, input.Command)
	case "localPlayback":
		plan, _, err := engine.downloads.localPlan(input.Drama.ID, input.Index)
		return plan, err
	case "catalog":
		return engine.nativeCatalog(ctx, input)
	case "cachedMany":
		return engine.cachedMany(input.Sources), nil
	case "cached":
		if !validNativeCategory(canonicalProviderSource(input.Source), input.Category) {
			return nil, errors.New("内容分类无效")
		}
		return engine.nativeCached(nativeCatalogKey(input.Source, input.Category)), nil
	case "categories":
		items, err := engine.nativeCategories(ctx, input.Source, input.Force)
		return map[string]any{"items": items}, err
	case "sourceStatus":
		return engine.sourceStatus(input.Source), nil
	case "sourceStatuses":
		return engine.sourceStatuses(input.Sources), nil
	case "sourceJob":
		return engine.startSourceTask(input.Source, input.Command, input.Drama)
	case "cancelSourceJob":
		return engine.cancelSourceTask(input.Source), nil
	case "cover":
		return engine.loadCover(ctx, input.Drama, input.Force)
	case "prepareCover":
		return engine.prepareCover(ctx, input.Drama)
	case "modules":
		return map[string]any{"items": engine.listModules()}, nil
	case "shadow":
		if input.Command == "list" || input.Source == "" {
			return map[string]any{"modes": shadowModes()}, nil
		}
		setShadowMode(input.Source, input.Command)
		return map[string]any{"source": input.Source, "mode": input.Command, "modes": shadowModes()}, nil
	case "probeSources":
		items := engine.probeSources(ctx, input.Sources, input.Force)
		return map[string]any{"items": items}, nil
	case "warmCatalogs":
		items := engine.warmCatalogs(ctx, input.Sources, input.Force)
		return map[string]any{"items": items}, nil
	case "installModule":
		status, err := engine.installModule(input.ModulePath)
		return status, err
	case "installFromRegistry":
		status, err := engine.InstallFromRegistry(registryInstallRequest{
			RegistryURL:  input.RegistryURL,
			ModuleID:     input.Source,
			IncludeAdult: input.IncludeAdult,
			Force:        input.Force,
		})
		return status, err
	case "removeModule":
		return true, engine.removeModule(input.Source)
	case "crossSource":
		return engine.nativeCrossSource(ctx, input)
	case "aggregateCatalog":
		return engine.nativeAggregateCatalog(ctx, input)
	case "aggregateCached":
		return engine.nativeAggregateCached(input), nil
	case "aggregateSources":
		return engine.nativeAggregateSources(ctx, input)
	case "catalogSnapshotInfo":
		return engine.nativeCatalogSnapshotInfo(ctx)
	case "flushCatalog":
		// 切后台/退出前把防抖窗口里的剧库立刻写掉。
		engine.mu.Lock()
		err := engine.flushCatalogSave()
		engine.mu.Unlock()
		if err != nil {
			return nil, err
		}
		return map[string]any{"ok": true}, nil
	case "syncCatalogSnapshot":
		return engine.nativeSyncCatalogSnapshot(ctx, input.Sources)
	// ---- 门面动作（S6）----
	// 这四个是 Flutter 唯一应该调用的入口：catalog / detail / stream / aggregate。
	// 其余 action 保留但视为内部实现，后续逐步迁走（见 ipc_surface.go 的登记表）。
	case "stream":
		switch strings.ToLower(strings.TrimSpace(input.Stage)) {
		case "fallback":
			playback, finish, err := engine.nativeBeginPlayback(ctx, input.Sequence)
			if err != nil {
				return nil, err
			}
			defer finish()
			return engine.nativeNextPlayback(playback, input.Session)
		default:
			playback, finish, err := engine.nativeBeginPlayback(ctx, input.Sequence)
			if err != nil {
				return nil, err
			}
			defer finish()
			return engine.nativeResolve(playback, input)
		}
	case "aggregate":
		switch strings.ToLower(strings.TrimSpace(input.Stage)) {
		case "sources":
			return engine.nativeAggregateSources(ctx, input)
		default:
			return engine.nativeAggregateCatalog(ctx, input)
		}
	case "detail":
		return engine.nativeDetail(ctx, input.Drama)
	case "metadata":
		return engine.nativeMetadata(ctx, input.Drama)
	case "resolve", "fallback":
		playback, finish, err := engine.nativeBeginPlayback(ctx, input.Sequence)
		if err != nil {
			return nil, err
		}
		defer finish()
		if input.Action == "fallback" {
			return engine.nativeNextPlayback(playback, input.Session)
		}
		return engine.nativeResolve(playback, input)
	case "cancelPlayback":
		engine.nativeCancelPlayback(input.Sequence)
		return true, nil
	case "release":
		engine.nativeReleasePlayback(input.Session)
		return true, nil
	default:
		return nil, errors.New("不支持的应用操作")
	}
}

func (engine *nativeEngine) nativeCatalog(ctx context.Context, input nativeInput) (nativeCatalogResult, error) {
	source := canonicalProviderSource(input.Source)
	if !isHuangguoProviderSource(source) {
		return nativeCatalogResult{}, errors.New("请选择有效站源")
	}
	// 已安装的模块接管同名站源，内置实现作为兜底。
	if moduleTakesOver(source) {
		items, more, err := engine.downloader.moduleCatalog(ctx, source, max(1, input.Page), strings.TrimSpace(input.Category), strings.TrimSpace(input.Query))
		if err != nil {
			return nativeCatalogResult{}, err
		}
		result := nativeCatalogResult{Items: []nativeDrama{}, Page: max(1, input.Page), HasMore: more}
		for _, drama := range items {
			result.Items = append(result.Items, nativeNormalize(drama))
		}
		return result, nil
	}
	if isRetiredSource(source) && moduleSource(source) == nil {
		return nativeCatalogResult{}, errSourceRetired
	}
	category := strings.TrimSpace(input.Category)
	if !validNativeCategory(source, category) {
		return nativeCatalogResult{}, errors.New("内容分类无效")
	}
	cacheKey := nativeCatalogKey(source, category)
	if err := ctx.Err(); err != nil {
		return nativeCatalogResult{}, err
	}
	page := max(1, input.Page)
	query := strings.TrimSpace(input.Query)
	if query == "" {
		if retried, err := engine.retryCatalogSave(); retried {
			cached := engine.nativeCached(cacheKey)
			if err != nil || len(cached.Items) > 0 {
				return cached, nil
			}
		}
	}
	if query == "" && page == 1 && !input.Force {
		cached := engine.nativeCached(cacheKey)
		if cached.Fresh {
			return cached, nil
		}
		// 有旧快照也先给：首屏不该等网络。
		// 只要快照还在「可接受陈旧」范围内就直接返回，刷新交给后台
		// warmCatalogs / 自动更新慢慢做；超过上限才同步联网，避免用户
		// 打开时看到几个月前的内容。
		if len(cached.Items) > 0 {
			engine.mu.Lock()
			updated := engine.catalogStates[cacheKey].UpdatedAt
			engine.mu.Unlock()
			if !updated.IsZero() &&
				time.Since(updated) < nativeCatalogStaleServeLimit {
				cached.Warning = joinNativeWarnings(cached.Warning, "已显示本地快照，正在后台更新剧库")
				cached.HasMore = true
				return cached, nil
			}
		}
	}
	d := engine.downloader
	result := nativeCatalogResult{Items: []nativeDrama{}, Page: page}
	if query != "" && moduleTakesOver(source) {
		items, more, err := d.moduleCatalog(ctx, source, page, category, query)
		if err != nil {
			return result, err
		}
		for _, drama := range items {
			result.Items = append(result.Items, nativeNormalize(drama))
		}
		result.HasMore = more
		return result, nil
	}
	if query != "" {
		// 搜索统一走 builtinCatalogTable：每个源在表里自己决定怎么搜。
		// 表里没有的源落到下面的本地搜索兜底。
		if outcome, handled, dispatchErr := runBuiltinCatalog(engine, d, ctx, source, page, category, query); handled {
			if dispatchErr != nil {
				return result, dispatchErr
			}
			if outcome.Page > 0 {
				result.Page = outcome.Page
			}
			if outcome.Warning != "" {
				result.Warning = outcome.Warning
			}
			result.HasMore = outcome.HasMore
			for _, drama := range outcome.Items {
				result.Items = append(result.Items, nativeNormalize(drama))
			}
			return result, nil
		}
	}

	if query != "" {
		engine.mu.Lock()
		items := append([]nativeDrama{}, engine.catalogs[source]...)
		engine.mu.Unlock()
		for _, item := range items {
			text := hongguoSearchText(item.Title + " " + item.Description + " " + strings.Join(item.Tags, " "))
			matches := true
			for _, word := range strings.Fields(query) {
				matches = matches && strings.Contains(text, hongguoSearchText(word))
			}
			if matches {
				result.Items = append(result.Items, item)
			}
		}
		result.LocalSearch = true
		return result, nil
	}
	unlock, lockErr := engine.lockSourceCatalog(ctx, source)
	if lockErr != nil {
		return nativeCatalogResult{}, lockErr
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	var items []Drama
	var err error
	{
		// 内置源的分发已从这里的 26 个 case 收敛到 builtin_catalog_dispatch.go
		// 的一张表。此处只做「查表 → 调用 → 合并结果」，不再出现任何站点分支。
		outcome, handled, dispatchErr := runBuiltinCatalog(engine, d, ctx, source, page, category, query)
		if handled {
			items = outcome.Items
			err = dispatchErr
			result.HasMore = outcome.HasMore
			if outcome.Warning != "" {
				result.Warning = outcome.Warning
			}
			if outcome.Page > 0 {
				result.Page = outcome.Page
			}
			if outcome.LocalSearch {
				result.LocalSearch = true
			}
			if outcome.Hongguo != nil {
				result.hongguo = outcome.Hongguo
			}
		} else {
			// 表外源：运行时加载的 JS 模块优先，其次运行时注入的自定义源。
			if moduleTakesOver(source) {
				items, result.HasMore, err = d.moduleCatalog(ctx, source, page, category, strings.TrimSpace(input.Query))
			} else if isCustomSource(source) && d.customSources != nil {
				cfg, ok := d.customSources.get(source)
				if !ok {
					return result, errors.New("自定义站源配置已失效，请重新添加")
				}
				switch cfg.Type {
				case "maccms":
					items, result.HasMore, err = d.fetchMaccmsCatalogPage(ctx, source, page, "")
				case "aes":
					items, result.HasMore, err = d.fetchCustomAESCatalogPage(ctx, cfg, page, category, "")
				default:
					return result, errors.New("自定义站源类型不支持")
				}
			}
		}
	}

	if err != nil && len(items) == 0 {
		return result, err
	}
	if len(items) == 0 && page == 1 && !isCustomSource(source) &&
		source != sourceYeguo && source != sourceDSD &&
		source != sourceHuangguoMain && source != sourceHuangguoSelected && source != sourceHuangguoWelfare {
		return result, errors.New("站源暂未返回剧集，请稍后刷新")
	}
	if err != nil {
		result.Warning = publicError(err).Error()
	}
	seen := map[string]bool{}
	for _, drama := range items {
		if drama.ID == "" || seen[drama.ID] {
			continue
		}
		seen[drama.ID] = true
		result.Items = append(result.Items, nativeNormalize(drama))
	}
	engine.saveCatalogCache(cacheKey, &result)
	engine.backfillCovers(source, result.Items)
	return result, nil
}

func (engine *nativeEngine) nativeDetail(ctx context.Context, drama nativeDrama) (any, error) {
	source, sourceID, valid := splitProviderDramaID(drama.ID)
	if !valid {
		return nil, errors.New("剧集信息无效，请刷新剧库")
	}
	if moduleTakesOver(source) {
		drama, chapters, err := engine.downloader.moduleDetail(ctx, source, sourceID)
		if err != nil {
			return nil, err
		}
		if len(chapters) == 0 {
			return nil, errors.New("该剧暂时没有可播放的分集")
		}
		normalized := nativeNormalize(drama)
		normalized.Source, normalized.SourceID, normalized.Episodes = source, sourceID, len(chapters)
		return map[string]any{"drama": normalized, "chapters": chapters, "warning": ""}, nil
	}
	var title string
	var raw Drama
	var chapters []Chapter
	var err error
	var enrichDrama func(d *Downloader, ctx context.Context, drama *nativeDrama)
	var episodesFromRaw bool
	if outcome, handled, dispatchErr := runBuiltinDetail(engine, engine.downloader, ctx, source, sourceID); handled {
		title, raw, chapters = outcome.Title, outcome.Raw, outcome.Chapters
		if outcome.Enrich != nil {
			enrichDrama = outcome.Enrich
		}
		if outcome.EpisodesFromRaw {
			episodesFromRaw = true
		}
		if dispatchErr != nil {
			return nil, dispatchErr
		}
	} else {
		// 表外源：运行时模块优先，其次运行时注入的自定义源，最后回落到旧版黄果取集。
		if moduleTakesOver(source) {
			raw, chapters, err = engine.downloader.moduleDetail(ctx, source, sourceID)
		} else if isCustomSource(source) && engine.downloader.customSources != nil {
			cfg, ok := engine.downloader.customSources.get(source)
			if !ok {
				return nil, errors.New("自定义站源配置已失效，请重新添加")
			}
			switch cfg.Type {
			case "maccms":
				raw, chapters, err = engine.downloader.fetchMaccmsDetail(ctx, source, sourceID)
			case "aes":
				raw, chapters, err = engine.downloader.fetchCustomAESDetail(ctx, cfg, sourceID)
			default:
				return nil, errors.New("自定义站源类型不支持")
			}
		} else {
			title, chapters, err = engine.downloader.GetHuangguoChapters(ctx, source, sourceID)
		}
	}
	if err != nil {
		return nil, err
	}
	if len(chapters) == 0 {
		return nil, errors.New("该剧暂时没有可播放的分集")
	}
	if title != "" && title != "短剧" {
		drama.Title = title
	}
	if raw.ID == drama.ID {
		drama = mergeNativeDrama(drama, nativeNormalize(raw))
	}
	if enrichDrama != nil {
		enrichDrama(engine.downloader, ctx, &drama)
	}
	drama.Source, drama.SourceID, drama.Episodes = source, sourceID, len(chapters)
	if episodesFromRaw {
		drama.Episodes = max(drama.Episodes, nativeNormalize(raw).Episodes)
	}
	warning := ""
	if err := engine.saveDetailMetadata(drama); err != nil {
		warning = err.Error()
	}
	return map[string]any{"drama": drama, "chapters": chapters, "warning": warning}, nil
}

func (engine *nativeEngine) nativeResolve(ctx context.Context, input nativeInput) (nativePlan, error) {
	if _, _, valid := splitProviderDramaID(input.Drama.ID); !valid {
		return nativePlan{}, errors.New("剧集信息无效")
	}
	if !input.Force && engine.downloads != nil {
		if plan, found, err := engine.downloads.localPlan(input.Drama.ID, input.Index); found || err != nil {
			if input.Action != "prepareHandoff" || !errors.Is(err, errNativeLocalFile) {
				return plan, err
			}
		}
	}
	task := Task{DramaID: input.Drama.ID, DramaTitle: input.Drama.Title, Chapter: input.Chapter, Index: input.Index}
	media, err := engine.downloader.resolveProviderMedia(ctx, task)
	if err != nil {
		return nativePlan{}, err
	}
	choice := nativePlaybackChoices(media, input.Quality)
	if series, video, valid := hongguoPlaybackIDs(task); valid {
		choice.danmakuSeries, choice.danmakuVideo = series, video
	}
	return engine.nativeOpenPlayback(ctx, choice)
}
