package core

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestFlutterCoreActionsStayOnFacades 静态扫描 lib/core_bridge.dart，
// 断言 Flutter 侧没有新增对旧 action 的调用。
//
// 这是 S6 的回归锁：门面动作只有四个，其余 action 一旦被前端重新引用，
// 收敛工作就会被悄悄撤销。
func TestFlutterCoreActionsStayOnFacades(t *testing.T) {
	path := filepath.Join("..", "..", "lib", "core_bridge.dart")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("跳过：读不到 %s（%v）", path, err)
	}
	// 仍允许前端调用的非门面动作（属下载/存储/弹幕等辅助能力）。
	allowed := map[string]bool{
		"lan": true, "prepareHandoff": true, "resourceSettings": true,
		"saveResourceSettings": true, "cancelRead": true, "preload": true,
		"danmaku": true, "warmCatalogs": true, "storage": true,
		"downloads": true, "enqueueDownloads": true, "controlDownloads": true,
		"controlDownloadBatch": true, "downloadDirectory": true, "moveDownloads": true,
		"workLease": true, "localPlayback": true, "cover": true, "prepareCover": true,
		"modules": true, "installModule": true, "installFromRegistry": true,
		"removeModule": true, "probeSources": true, "sourceStatuses": true,
		"sourceStatus": true, "sourceJob": true, "cancelSourceJob": true,
		"cachedMany": true, "cached": true, "categories": true,
		"catalogSnapshotInfo": true, "syncCatalogSnapshot": true,
		"crossSource": true, "metadata": true, "cancelPlayback": true,
		"release": true, "recommendations": true, "cachedRecommendations": true,
		"rankingBoards": true, "rankings": true, "suggestions": true,
		"registerCustomSources": true, "listCustomSources": true,
		"updateSystemProxy": true, "sourceProbe": true,
	}
	declared := map[string]bool{}
	for _, action := range append(IPCFacadeActions(), IPCInternalActions()...) {
		declared[action] = true
	}
	pattern := regexp.MustCompile(`'action':\s*'([a-zA-Z]+)'`)
	unknown := map[string]bool{}
	for _, match := range pattern.FindAllStringSubmatch(string(body), -1) {
		action := match[1]
		if IsIPCFacadeAction(action) || allowed[action] {
			continue
		}
		if !declared[action] {
			unknown[action] = true
		}
	}
	for action := range unknown {
		t.Errorf("core_bridge.dart 调用了未登记的 action %q", action)
	}
}
