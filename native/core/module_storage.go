package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// 模块私有存储：按模块 id 隔离，跨调用保留，带 TTL 与配额。
// 用途是令牌、设备标识、发布页入口这类「需要过期」的数据。
// 与 moduleCacheEntry 的区别：那个属于单次调用内的缓存；
// 这里的存储跟随模块生命周期，卸载模块时清空。

const (
	moduleStorageKeyMax     = 128
	moduleStorageValueMax   = 64 << 10  // 单值 64 KB
	moduleStorageTotalBytes = 256 << 10 // 每模块 256 KB
	moduleStorageEntryMax   = 128       // 每模块 128 条
	moduleStorageTTLMax     = 30 * 24 * time.Hour
)

type moduleStorageEntry struct {
	value   string
	expires time.Time
}

type moduleStorage struct {
	mu      sync.Mutex
	entries map[string]moduleStorageEntry
}

var moduleStorageRegistry = struct {
	sync.Mutex
	byModule map[string]*moduleStorage
}{byModule: map[string]*moduleStorage{}}

// moduleStorageFor 返回某个模块的存储，不存在则创建。
func moduleStorageFor(id string) *moduleStorage {
	id = strings.ToLower(strings.TrimSpace(id))
	moduleStorageRegistry.Lock()
	defer moduleStorageRegistry.Unlock()
	store, ok := moduleStorageRegistry.byModule[id]
	if !ok {
		store = &moduleStorage{entries: map[string]moduleStorageEntry{}}
		moduleStorageRegistry.byModule[id] = store
	}
	return store
}

// DropModuleStorage 清空并移除某个模块的存储；卸载模块时调用。
func DropModuleStorage(id string) {
	id = strings.ToLower(strings.TrimSpace(id))
	moduleStorageRegistry.Lock()
	delete(moduleStorageRegistry.byModule, id)
	moduleStorageRegistry.Unlock()
}

// Get 读取键值；键不存在或已过期返回 ("", false)。
func (store *moduleStorage) Get(key string) (string, bool) {
	key, err := moduleStorageKey(key)
	if err != nil {
		return "", false
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	entry, ok := store.entries[key]
	if !ok {
		return "", false
	}
	if !entry.expires.IsZero() && time.Now().After(entry.expires) {
		delete(store.entries, key)
		return "", false
	}
	return entry.value, true
}

// Set 写入键值；ttl 为 0 表示不过期，超过 30 天按 30 天截断。
func (store *moduleStorage) Set(key, value string, ttl time.Duration) error {
	key, err := moduleStorageKey(key)
	if err != nil {
		return err
	}
	if len(value) > moduleStorageValueMax {
		return fmt.Errorf("存储值超过 %d KB", moduleStorageValueMax>>10)
	}
	if ttl < 0 {
		return errors.New("存储过期时间不能为负数")
	}
	if ttl > moduleStorageTTLMax {
		ttl = moduleStorageTTLMax
	}
	var expires time.Time
	if ttl > 0 {
		expires = time.Now().Add(ttl)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeLocked(time.Now())
	store.entries[key] = moduleStorageEntry{value: value, expires: expires}
	if err := store.enforceLocked(); err != nil {
		delete(store.entries, key)
		return err
	}
	return nil
}

// Delete 删除键；键非法时静默忽略，不向模块抛错。
func (store *moduleStorage) Delete(key string) {
	key, err := moduleStorageKey(key)
	if err != nil {
		return
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.entries, key)
}

// Clear 清空全部键值。
func (store *moduleStorage) Clear() {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.entries = map[string]moduleStorageEntry{}
}

// Bytes 返回当前占用字节数（按 key+value 估算），供配额诊断。
func (store *moduleStorage) Bytes() int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.sizeLocked()
}

// Keys 按字典序返回未过期的键，供调试与诊断。
func (store *moduleStorage) Keys() []string {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.purgeLocked(time.Now())
	out := make([]string, 0, len(store.entries))
	for key := range store.entries {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (store *moduleStorage) purgeLocked(now time.Time) {
	for key, entry := range store.entries {
		if !entry.expires.IsZero() && now.After(entry.expires) {
			delete(store.entries, key)
		}
	}
}

// enforceLocked 在超出条目数或总字节时，按「最早过期」顺序淘汰。
// 没有过期时间的条目永不淘汰——它们通常是模块当前正在用的令牌，
// 静默删掉会造成「刚登录就掉线」这类难查的问题。
func (store *moduleStorage) enforceLocked() error {
	for len(store.entries) > moduleStorageEntryMax || store.sizeLocked() > moduleStorageTotalBytes {
		victim := ""
		var victimExpiry time.Time
		for key, entry := range store.entries {
			if entry.expires.IsZero() {
				continue
			}
			if victim == "" || entry.expires.Before(victimExpiry) {
				victim, victimExpiry = key, entry.expires
			}
		}
		if victim == "" {
			return fmt.Errorf("模块存储已满（%d 条 / %d KB），请清理后再写入", len(store.entries), store.sizeLocked()>>10)
		}
		delete(store.entries, victim)
	}
	return nil
}

func (store *moduleStorage) sizeLocked() int {
	total := 0
	for key, entry := range store.entries {
		total += len(key) + len(entry.value)
	}
	return total
}

// moduleStorageKey 归一化并校验键名。
func moduleStorageKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("存储键不能为空")
	}
	if len(key) > moduleStorageKeyMax {
		return "", fmt.Errorf("存储键超过 %d 个字符", moduleStorageKeyMax)
	}
	for _, symbol := range key {
		if symbol < 0x20 || symbol == 0x7f {
			return "", errors.New("存储键不能包含控制字符")
		}
	}
	return key, nil
}
