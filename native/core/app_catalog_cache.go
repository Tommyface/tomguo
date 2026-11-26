package core

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const nativeCatalogTTL = 15 * time.Minute

// nativeCatalogStaleServeLimit 是「旧快照仍然先给用户看」的上限。
// 超过这个时长就同步联网刷新，避免首屏长期停在陈旧内容上。
const nativeCatalogStaleServeLimit = 6 * time.Hour

type nativeCatalogState struct {
	UpdatedAt time.Time `json:"updatedAt"`
	Page      int       `json:"page"`
	HasMore   bool      `json:"hasMore"`
	Warning   string    `json:"warning,omitempty"`
}

type nativeCatalogDisk struct {
	Version         int                                  `json:"version"`
	Catalogs        map[string][]nativeDrama             `json:"catalogs"`
	States          map[string]nativeCatalogState        `json:"states"`
	Categories      map[string][]nativeCategory          `json:"categories,omitempty"`
	HongguoApp      *hongguoCatalogState                 `json:"hongguoApp,omitempty"`
	Recommendations map[string]nativeRecommendationState `json:"recommendations,omitempty"`
}

// loadCatalogCache 读取剧库缓存。
//
// 优先读按源分片（catalogs/<source>.json）；分片目录不存在时回落到旧的单文件
// catalogs.json，保证升级后第一次启动不丢历史缓存。
func (engine *nativeEngine) loadCatalogCache() {
	if engine.loadCatalogShards() {
		return
	}
	file, err := os.Open(filepath.Join(engine.directory, "catalogs.json"))
	if err != nil {
		return
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, nativeCatalogMaxBytes+1))
	if err != nil || len(body) > nativeCatalogMaxBytes {
		return
	}
	var disk nativeCatalogDisk
	if json.Unmarshal(body, &disk) == nil && (disk.Version == 2 || disk.Version == 3) {
		if disk.Catalogs != nil {
			engine.catalogs = disk.Catalogs
			for _, items := range engine.catalogs {
				for index := range items {
					items[index] = migrateNativeDrama(items[index])
				}
			}
		}
		if disk.States != nil {
			engine.catalogStates = disk.States
		}
		engine.categoryOptions = disk.Categories
		engine.recommendations = make(map[string]nativeRecommendationState)
		for genre, state := range disk.Recommendations {
			if genre == state.Query.Genre && state.Query.validate() == nil && state.Page > 0 {
				engine.recommendations[genre] = state
			}
		}
		engine.hongguoCatalog = cloneHongguoCatalogState(disk.HongguoApp)
		if engine.downloader != nil {
			engine.downloader.restoreHongguoCatalog(disk.HongguoApp)
			if disk.HongguoApp != nil {
				engine.hongguoCatalog = engine.downloader.hongguoCatalogSnapshot()
			}
		}
		return
	}
	var legacy map[string][]nativeDrama
	if json.Unmarshal(body, &legacy) == nil && legacy != nil {
		engine.catalogs = legacy
		for _, items := range engine.catalogs {
			for index := range items {
				items[index] = migrateNativeDrama(items[index])
			}
		}
	}
}

func migrateNativeDrama(drama nativeDrama) nativeDrama {
	if drama.MetadataSchema == 0 && drama.Source == sourceHuangdou && drama.VIP != nil && !*drama.VIP {
		drama.VIP = nil
	}
	drama.MetadataSchema = 1
	return drama
}

// cachedMany 一次取多个站源的目录缓存。首屏 read-through 场景下
// 每源一次 cached IPC 会把首帧拆成 N 次调用；合并成单次后，
// 首屏只读一次内存就拿到全部卡片。
func (engine *nativeEngine) cachedMany(sources []string) map[string]nativeCatalogResult {
	out := make(map[string]nativeCatalogResult, len(sources))
	engine.mu.Lock()
	defer engine.mu.Unlock()
	for _, raw := range sources {
		source := canonicalProviderSource(raw)
		if source == "" {
			continue
		}
		if _, ok := out[source]; ok {
			continue
		}
		items := append([]nativeDrama{}, engine.catalogs[source]...)
		for index := range items {
			items[index].Cover = repairLegacyCoverURL(items[index])
		}
		state, found := engine.catalogStates[source]
		age := time.Since(state.UpdatedAt)
		result := nativeCatalogResult{
			Items: items, Page: max(1, state.Page), HasMore: !found || state.HasMore,
			Fresh:   len(items) > 0 && state.Warning == "" && !state.UpdatedAt.IsZero() && age >= 0 && age < nativeCatalogTTL,
			Warning: state.Warning,
		}
		if engine.catalogSaveError != nil {
			result.Warning = joinNativeWarnings(result.Warning, engine.catalogSaveError.Error())
			result.Fresh, result.HasMore = false, true
		}
		out[source] = result
	}
	return out
}

func (engine *nativeEngine) nativeCached(source string) nativeCatalogResult {
	source = canonicalProviderSource(source)
	engine.mu.Lock()
	defer engine.mu.Unlock()
	items := append([]nativeDrama{}, engine.catalogs[source]...)
	for index := range items {
		items[index].Cover = repairLegacyCoverURL(items[index])
	}
	state, found := engine.catalogStates[source]
	age := time.Since(state.UpdatedAt)
	result := nativeCatalogResult{
		Items: items, Page: max(1, state.Page), HasMore: !found || state.HasMore,
		Fresh:   len(items) > 0 && state.Warning == "" && !state.UpdatedAt.IsZero() && age >= 0 && age < nativeCatalogTTL,
		Warning: state.Warning,
	}
	if engine.catalogSaveError != nil {
		result.Warning = joinNativeWarnings(result.Warning, engine.catalogSaveError.Error())
		result.saveError = engine.catalogSaveError
		result.Fresh, result.HasMore = false, true
	}
	return result
}

func mergeNativeCatalog(first, second []nativeDrama) []nativeDrama {
	// 与 ai0by 快照对齐：first 自身也可能含重复 id（多轮并集 / 增量合并产物），
	// 先内部去重再并 second，避免同一部剧出现两张卡。
	items := make([]nativeDrama, 0, len(first)+len(second))
	indices := make(map[string]int, len(first)+len(second))
	for _, item := range first {
		if item.ID == "" {
			items = append(items, item)
			continue
		}
		if index, found := indices[item.ID]; found {
			items[index] = mergeNativeDrama(items[index], item)
			continue
		}
		indices[item.ID] = len(items)
		items = append(items, item)
	}
	for _, item := range second {
		if item.ID == "" {
			items = append(items, item)
			continue
		}
		if index, found := indices[item.ID]; found {
			items[index] = mergeNativeDrama(items[index], item)
		} else {
			indices[item.ID] = len(items)
			items = append(items, item)
		}
	}
	return items
}

func mergeNativeDrama(previous, fresh nativeDrama) nativeDrama {
	previous, fresh = migrateNativeDrama(previous), migrateNativeDrama(fresh)
	if previous.ID != "" && fresh.ID != previous.ID {
		return fresh
	}
	if fresh.Source == "" {
		fresh.Source = previous.Source
	}
	if fresh.SourceID == "" {
		fresh.SourceID = previous.SourceID
	}
	if fresh.Title == "" || fresh.Title == "短剧" {
		fresh.Title = previous.Title
	}
	if fresh.Description == "" {
		fresh.Description = previous.Description
	}
	if fresh.Cover == "" {
		fresh.Cover = previous.Cover
	}
	if fresh.Category == "" || nativeGenericCategory(fresh.Category) && previous.Category != "" && !nativeGenericCategory(previous.Category) {
		fresh.Category = previous.Category
	}
	if fresh.Episodes == 0 {
		fresh.Episodes = previous.Episodes
	}
	if fresh.VIP == nil {
		fresh.VIP = previous.VIP
	}
	if fresh.Heat == "" {
		fresh.Heat = previous.Heat
	}
	if fresh.Views == "" {
		fresh.Views = previous.Views
	}
	if fresh.OnlineDate == "" {
		fresh.OnlineDate = previous.OnlineDate
	}
	if len(fresh.Tags) == 0 {
		fresh.Tags = previous.Tags
	}
	if fresh.ReleaseStatus == "" || fresh.ReleaseStatus == "unknown" {
		fresh.ReleaseStatus = previous.ReleaseStatus
	}
	return fresh
}

func nativeGenericCategory(category string) bool {
	switch category {
	case "短剧", "真人剧", "漫剧", "AI剧", "AI 剧", "AI短剧", "AI 短剧", "AI漫剧", "AI 漫剧":
		return true
	}
	return false
}

func (engine *nativeEngine) saveCatalogCache(source string, result *nativeCatalogResult) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	previous := engine.catalogs[source]
	state := engine.catalogStates[source]
	var items []nativeDrama
	if result.Page == 1 {
		items = result.Items
		if len(previous) > 0 {
			old := make(map[string]nativeDrama, len(previous))
			for _, item := range previous {
				old[item.ID] = item
			}
			for index, item := range items {
				items[index] = mergeNativeDrama(old[item.ID], item)
			}
			fresh := make(map[string]bool, len(items))
			for _, item := range items {
				fresh[item.ID] = true
			}
			for _, item := range previous {
				if !fresh[item.ID] {
					items = append(items, item)
				}
			}
			result.Items = items
			result.Page = max(1, state.Page)
			if source != sourceHongguo && state.Page > 1 {
				result.HasMore = state.HasMore
			}
		}
	} else {
		items = mergeNativeCatalog(previous, result.Items)
		if result.Page < state.Page {
			result.Page, result.HasMore = state.Page, state.HasMore
		}
	}
	engine.catalogs[source] = items
	if result.hongguo != nil {
		engine.hongguoCatalog = cloneHongguoCatalogState(result.hongguo)
	}
	if result.Warning != "" {
		result.Page = max(1, state.Page)
		result.HasMore = true
	}
	state.Page, state.HasMore = result.Page, result.HasMore
	state.Warning = result.Warning
	if result.Warning == "" {
		state.UpdatedAt = time.Now()
		result.Fresh = true
	}
	engine.catalogStates[source] = state
	if base, _, categorized := strings.Cut(source, "|"); categorized {
		engine.catalogs[base] = mergeNativeCatalog(engine.catalogs[base], items)
	}
	// 不在这里同步落盘：一轮刷新会连续抓多个源，交给防抖定时器合并。
	engine.scheduleCatalogSaveLocked()
	result.saveError = engine.catalogSaveError
	if result.saveError != nil {
		result.Warning = joinNativeWarnings(result.Warning, result.saveError.Error())
		result.Fresh, result.HasMore = false, true
	}
	return result.saveError
}

// nativeCatalogSaveDelay 是剧库落盘的防抖窗口。
//
// 一轮剧库刷新里每个站源抓完都会写一次，源越多写得越多。合并成一个窗口
// 之后，连续抓取只在安静下来时落一次盘；加上写文件放到锁外，首页的
// 读取类 IPC 不会被 IO 堵住。
const nativeCatalogSaveDelay = 1200 * time.Millisecond

// scheduleCatalogSaveLocked 申请一次延迟落盘；调用方必须持有 engine.mu。
func (engine *nativeEngine) scheduleCatalogSaveLocked() {
	if engine.catalogSaveTimer != nil {
		engine.catalogSaveTimer.Stop()
	}
	engine.catalogSaveTimer = time.AfterFunc(nativeCatalogSaveDelay, func() {
		engine.mu.Lock()
		engine.catalogSaveTimer = nil
		// 分片落盘：每个源一个文件，源数据互不影响。
		err := engine.saveCatalogShards(false)
		engine.mu.Unlock()
		engine.mu.Lock()
		engine.catalogSaveError = nativeSaveError("剧库", err)
		if err == nil {
			engine.finishCatalogSaveLocked()
		} else {
			engine.markCatalogSavePendingLocked()
		}
		engine.mu.Unlock()
	})
}

// flushCatalogSave 立即落盘并取消待执行的防抖定时器。
// 切后台、退出前调用，避免最后一批更新丢在定时器里。
func (engine *nativeEngine) flushCatalogSave() error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.catalogSaveTimer != nil {
		engine.catalogSaveTimer.Stop()
		engine.catalogSaveTimer = nil
	}
	return engine.writeCatalogDiskLocked()
}

// marshalCatalogDiskLocked 序列化剧库快照；调用方必须持有 engine.mu。
func (engine *nativeEngine) marshalCatalogDiskLocked() ([]byte, error) {
	body, err := json.Marshal(nativeCatalogDisk{Version: 3, Catalogs: engine.catalogs, States: engine.catalogStates, Categories: engine.categoryOptions, HongguoApp: engine.hongguoCatalog, Recommendations: engine.recommendations})
	if err == nil && len(body) > nativeCatalogMaxBytes {
		err = errNativeCatalogLimit
	}
	return body, err
}

func (engine *nativeEngine) writeCatalogDiskLocked() error {
	// force=true：切后台/退出前必须把当前状态完整写下去。
	err := engine.saveCatalogShards(true)
	engine.catalogSaveError = nativeSaveError("剧库", err)
	if err == nil {
		engine.finishCatalogSaveLocked()
	} else {
		engine.markCatalogSavePendingLocked()
	}
	return engine.catalogSaveError
}

func writeNativeCacheFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".cache-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	_, writeErr := temporary.Write(data)
	if writeErr == nil {
		writeErr = temporary.Sync()
	}
	closeErr := temporary.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(temporary.Name(), path)
}

func (engine *nativeEngine) saveDetailMetadata(drama nativeDrama) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	changed := false
	for _, items := range engine.catalogs {
		for index := range items {
			if items[index].ID == drama.ID {
				items[index] = mergeNativeDrama(items[index], drama)
				changed = true
			}
		}
	}
	if changed {
		return engine.writeCatalogDiskLocked()
	}
	return nil
}

// loadCatalogShards 从分片目录恢复剧库；返回 true 表示至少恢复了一个源。
func (engine *nativeEngine) loadCatalogShards() bool {
	directory := engine.directory
	names := listCatalogShards(directory)
	if len(names) == 0 {
		return false
	}
	loaded := 0
	for _, name := range names {
		shard, ok := readCatalogShard(directory, name)
		if !ok {
			// 单个分片损坏只丢这个源，不影响其余源。
			continue
		}
		source := canonicalProviderSource(shard.Source)
		if source == "" {
			continue
		}
		items := make([]nativeDrama, len(shard.Items))
		for index, item := range shard.Items {
			items[index] = migrateNativeDrama(item)
		}
		engine.catalogs[source] = items
		engine.catalogStates[source] = shard.State
		if len(shard.Categories) > 0 {
			engine.categoryOptions[source] = shard.Categories
		}
		if shard.HongguoApp != nil {
			engine.hongguoCatalog = cloneHongguoCatalogState(shard.HongguoApp)
			if engine.downloader != nil {
				engine.downloader.restoreHongguoCatalog(shard.HongguoApp)
			}
		}
		loaded++
	}
	return loaded > 0
}

// saveCatalogShards 按源写分片，并更新索引。调用方必须持有 engine.mu。
//
// 只写「内容有变化」的源：分片文件按 source 命名，重写未变化的源没有意义，
// 而且源越多写盘越慢。变化判定用 bytes 长度 + 状态时间戳，代价远低于序列化。
func (engine *nativeEngine) saveCatalogShards(force bool) error {
	directory := engine.directory
	index := readCatalogShardIndex(directory)
	updated := make(map[string]catalogShardMeta, len(engine.catalogs))
	var firstErr error
	for source, items := range engine.catalogs {
		state := engine.catalogStates[source]
		meta, seen := index.Shards[safeShardName(source)]
		if !force && seen && meta.SavedAt.Equal(state.UpdatedAt) && meta.SavedAt.After(time.Time{}) {
			updated[safeShardName(source)] = meta
			continue
		}
		shard := catalogShard{
			Source: source,
			Items:  items,
			State:  state,
		}
		if categories, ok := engine.categoryOptions[source]; ok {
			shard.Categories = categories
		}
		if source == sourceHongguo && engine.hongguoCatalog != nil {
			shard.HongguoApp = engine.hongguoCatalog
		}
		written, err := writeCatalogShard(directory, shard)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		updated[safeShardName(source)] = written
	}
	if err := writeCatalogShardIndex(directory, updated); err != nil && firstErr == nil {
		firstErr = err
	}
	// 清理长期不再更新的分片（例如源被删除后残留的目录）。
	keep := make(map[string]bool, len(engine.catalogs))
	for source := range engine.catalogs {
		keep[safeShardName(source)] = true
	}
	pruneCatalogShards(directory, keep, 30*24*time.Hour)
	return firstErr
}
