package core

import (
	"context"
	"time"
)

const nativeCoverBackfillLimit = 6
const nativeCoverBackfillTTL = 6 * time.Hour

// pendingCoverBackfill 挑出缺少海报、且最近没有补过的剧集。
// 详情页通常带 og:image，补一次就能长期显示，不必每屏都重试。
func (engine *nativeEngine) pendingCoverBackfill(items []nativeDrama) []string {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if engine.coverBackfill == nil {
		engine.coverBackfill = map[string]time.Time{}
	}
	now := time.Now()
	picked := make([]string, 0, nativeCoverBackfillLimit)
	for _, item := range items {
		if len(picked) >= nativeCoverBackfillLimit {
			break
		}
		if item.ID == "" || item.Cover != "" {
			continue
		}
		if attempted, found := engine.coverBackfill[item.ID]; found && now.Sub(attempted) < nativeCoverBackfillTTL {
			continue
		}
		engine.coverBackfill[item.ID] = now
		picked = append(picked, item.ID)
	}
	return picked
}

// backfillCovers 为缺海报的条目补详情元数据：详情页解析到 og:image 后
// 通过 saveDetailMetadata 回写剧库缓存，下一屏就有图。
func (engine *nativeEngine) backfillCovers(source string, items []nativeDrama) {
	if len(items) == 0 || isRetiredSource(source) {
		return
	}
	ids := engine.pendingCoverBackfill(items)
	if len(ids) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		for _, id := range ids {
			if ctx.Err() != nil {
				return
			}
			drama, err := engine.nativeDetail(ctx, nativeDrama{ID: id, Source: source})
			if err != nil {
				continue
			}
			payload, ok := drama.(map[string]any)
			if !ok {
				continue
			}
			found, ok := payload["drama"].(nativeDrama)
			if !ok || found.Cover == "" || found.Cover == "about:blank" {
				continue
			}
			_ = engine.saveDetailMetadata(found)
		}
	}()
}
