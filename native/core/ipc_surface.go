package core

import "strings"

// IPC 表面（S6）。
//
// 历史上 Flutter 通过 NativeRequest 调几十个 action，主程序内部实现一改动就可能
// 影响前端调用。S6 的目标是把前端可见面收敛成四个门面动作：
//
//	catalog   目录与搜索
//	detail    详情
//	stream    播放地址（stage=resolve/fallback）
//	aggregate 跨源聚合（stage=page/sources，mode=instant/refresh/force）
//
// 其余 action 仍是内部实现，逐步迁往门面。本文件登记这张表，
// 让「哪些还在门面之外」这件事可被测试统计，而不是靠人记。

// ipcFacadeActions 是 Flutter 唯一应该调用的动作。
var ipcFacadeActions = []string{"catalog", "detail", "stream", "aggregate"}

// ipcInternalActions 是仍然暴露、但视为内部实现的动作。
// 每一条都应在后续阶段被门面取代；测试会锁住「只减不增」。
var ipcInternalActions = []string{
	// initialize 在分发表之前就被拦截（引擎启动握手），不参与 action switch。
	"initialize", "lan", "registerCustomSources", "listCustomSources", "updateSystemProxy",
	"resourceSettings", "saveResourceSettings", "controlDownloadBatch", "preload",
	"prepareHandoff", "danmaku", "cancelRead", "recommendations",
	"cachedRecommendations", "rankingBoards", "rankings", "suggestions",
	"downloadDirectory", "storage", "moveDownloads", "workLease", "downloads",
	"enqueueDownloads", "controlDownloads", "localPlayback", "cachedMany", "cached",
	"categories", "sourceStatus", "sourceStatuses", "sourceJob", "cancelSourceJob",
	"cover", "prepareCover", "modules", "probeSources", "warmCatalogs",
	"installModule", "installFromRegistry", "removeModule", "crossSource",
	"aggregateCatalog", "aggregateCached", "aggregateSources", "catalogSnapshotInfo",
	"flushCatalog", "syncCatalogSnapshot", "metadata", "resolve", "fallback",
	"cancelPlayback", "release",
}

// IsIPCFacadeAction 报告某个 action 是否属于四个门面之一。
func IsIPCFacadeAction(action string) bool {
	action = strings.ToLower(strings.TrimSpace(action))
	for _, facade := range ipcFacadeActions {
		if facade == action {
			return true
		}
	}
	return false
}

// IPCFacadeActions 返回门面动作清单（副本，调用方可安全修改）。
func IPCFacadeActions() []string {
	out := make([]string, len(ipcFacadeActions))
	copy(out, ipcFacadeActions)
	return out
}

// IPCInternalActions 返回仍在门面之外的动作清单。
func IPCInternalActions() []string {
	out := make([]string, len(ipcInternalActions))
	copy(out, ipcInternalActions)
	return out
}
