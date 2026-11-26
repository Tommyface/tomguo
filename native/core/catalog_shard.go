package core

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// 按源分片持久化（S5）。
//
// 旧实现把所有站源的剧库写进一个 catalogs.json：
//   - 单文件触达 32 MiB 上限后整库停止保存；
//   - 一个源的数据损坏，整份缓存都无法解析；
//   - 每次防抖落盘都要重写全部源，源越多写越慢。
//
// 新实现为每个源写一个分片文件 catalogs/<source>.json，另有一份 index.json 记录
// 各分片的写入时间与字节数。读的时候逐分片解析，坏一个只丢一个源的缓存。

const catalogShardDirName = "catalogs"
const catalogShardIndexName = "index.json"
const catalogShardVersion = 1

// catalogShard 是一个源的分片文件内容。
type catalogShard struct {
	Version    int                  `json:"version"`
	Source     string               `json:"source"`
	SavedAt    time.Time            `json:"savedAt"`
	Items      []nativeDrama        `json:"items"`
	State      nativeCatalogState   `json:"state"`
	Categories []nativeCategory     `json:"categories,omitempty"`
	HongguoApp *hongguoCatalogState `json:"hongguoApp,omitempty"`
}

// catalogShardIndex 记录全部分片的元信息，便于启动时快速判断哪些需要加载。
type catalogShardIndex struct {
	Version int                         `json:"version"`
	SavedAt time.Time                   `json:"savedAt"`
	Shards  map[string]catalogShardMeta `json:"shards"`
}

type catalogShardMeta struct {
	Bytes   int64     `json:"bytes"`
	SavedAt time.Time `json:"savedAt"`
}

func catalogShardDir(directory string) string {
	return filepath.Join(directory, catalogShardDirName)
}

func catalogShardPath(directory, source string) string {
	return filepath.Join(catalogShardDir(directory), safeShardName(source)+".json")
}

// safeShardName 把源 id 变成安全的文件名，避免路径穿越与非法字符。
func safeShardName(source string) string {
	source = strings.ToLower(strings.TrimSpace(source))
	var builder strings.Builder
	for _, r := range source {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			builder.WriteRune(r)
		case r == '|':
			builder.WriteRune('_')
		}
	}
	name := builder.String()
	if name == "" {
		return "unknown"
	}
	return name
}

// writeCatalogShard 把单个源写成独立分片。调用方负责并发控制。
func writeCatalogShard(directory string, shard catalogShard) (catalogShardMeta, error) {
	shard.Version = catalogShardVersion
	if shard.SavedAt.IsZero() {
		shard.SavedAt = time.Now()
	}
	body, err := json.Marshal(shard)
	if err != nil {
		return catalogShardMeta{}, err
	}
	path := catalogShardPath(directory, shard.Source)
	if err := writeNativeCacheFile(path, body); err != nil {
		return catalogShardMeta{}, err
	}
	return catalogShardMeta{Bytes: int64(len(body)), SavedAt: shard.SavedAt}, nil
}

// readCatalogShard 读取单个源的分片；分片缺失或损坏都返回 ok=false，由调用方跳过。
func readCatalogShard(directory, source string) (catalogShard, bool) {
	path := catalogShardPath(directory, source)
	file, err := os.Open(path)
	if err != nil {
		return catalogShard{}, false
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, nativeCatalogMaxBytes+1))
	if err != nil || int64(len(body)) > nativeCatalogMaxBytes {
		return catalogShard{}, false
	}
	var shard catalogShard
	if json.Unmarshal(body, &shard) != nil || shard.Version != catalogShardVersion {
		return catalogShard{}, false
	}
	return shard, true
}

// writeCatalogShardIndex 原子写索引文件。
func writeCatalogShardIndex(directory string, shards map[string]catalogShardMeta) error {
	index := catalogShardIndex{Version: catalogShardVersion, SavedAt: time.Now(), Shards: shards}
	body, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return writeNativeCacheFile(filepath.Join(catalogShardDir(directory), catalogShardIndexName), body)
}

// readCatalogShardIndex 读索引；缺失时返回空索引，不视为错误。
func readCatalogShardIndex(directory string) catalogShardIndex {
	body, err := os.ReadFile(filepath.Join(catalogShardDir(directory), catalogShardIndexName))
	if err != nil {
		return catalogShardIndex{Version: catalogShardVersion, Shards: map[string]catalogShardMeta{}}
	}
	var index catalogShardIndex
	if json.Unmarshal(body, &index) != nil {
		return catalogShardIndex{Version: catalogShardVersion, Shards: map[string]catalogShardMeta{}}
	}
	if index.Shards == nil {
		index.Shards = map[string]catalogShardMeta{}
	}
	return index
}

// listCatalogShards 列出现存分片对应的源 id（不依赖索引，索引丢失也能恢复）。
func listCatalogShards(directory string) []string {
	entries, err := os.ReadDir(catalogShardDir(directory))
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || name == catalogShardIndexName {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(names)
	return names
}

// pruneCatalogShards 删除超过保留期且不在 keep 集合里的分片，返回删除数量。
func pruneCatalogShards(directory string, keep map[string]bool, limit time.Duration) int {
	removed := 0
	for _, name := range listCatalogShards(directory) {
		if keep[name] {
			continue
		}
		path := filepath.Join(catalogShardDir(directory), name+".json")
		info, err := os.Stat(path)
		if err != nil || time.Since(info.ModTime()) < limit {
			continue
		}
		if os.Remove(path) == nil {
			removed++
		}
	}
	return removed
}

var errCatalogShardEmpty = errors.New("分片没有可保存内容")
