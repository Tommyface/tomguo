package core

import (
	"testing"
)

func TestNormalizeAggregateTitle(t *testing.T) {
	cases := map[string]string{
		"  新秘密教学  全集 ": "新秘密教学",
		"《同学的离异妈妈》":    "同学的离异妈妈",
		"母上攻略（高清完整版）":  "母上攻略",
		"AI  漫剧-测试":    "ai漫剧测试",
	}
	for input, expected := range cases {
		if got := normalizeAggregateTitle(input); got != expected {
			t.Errorf("normalize(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestInferContentKindPriority(t *testing.T) {
	cases := []struct {
		category string
		title    string
		want     string
	}{
		{"ai-manju", "随便", kindAIManhua},
		{"", "某某 AI 魔改短剧", kindAIMogai},
		{"ai-huanlian", "", kindAIHuanlian},
		{"ai-duanju", "", kindAIDuanju},
		{"", "花絮：拍摄幕后", kindVideo},
		{"短剧", "", kindLiveAction},
		{"", "无名作品", kindShortDrama},
	}
	for _, tc := range cases {
		drama := nativeDrama{Category: tc.category, Title: tc.title}
		if got := inferContentKind(drama); got != tc.want {
			t.Errorf("kind(%q,%q) = %q, want %q", tc.category, tc.title, got, tc.want)
		}
	}
}

func TestAggregateByTitleMergesSources(t *testing.T) {
	perSource := map[string][]nativeDrama{
		"chengguo": {
			{ID: "chengguo:dj-1", Source: "chengguo", Title: "新秘密教学 全集", Episodes: 80, Category: "AI短剧"},
		},
		"xingya": {
			{ID: "xingya:x-1", Source: "xingya", Title: "新秘密教学", Episodes: 60, Category: "短剧"},
		},
	}
	items := aggregateByTitle(perSource, false)
	if len(items) != 1 {
		t.Fatalf("merged items = %d, want 1", len(items))
	}
	if items[0].SourceCount != 2 || items[0].Episodes != 80 {
		t.Fatalf("aggregate = %+v", items[0])
	}
	if items[0].ContentKind != kindAIDuanju {
		t.Fatalf("kind = %q, want %q", items[0].ContentKind, kindAIDuanju)
	}
	if items[0].Sources[0].Source != "chengguo" {
		t.Fatalf("top source = %q", items[0].Sources[0].Source)
	}
}

func TestAggregateByTitleSeparatesAdultView(t *testing.T) {
	perSource := map[string][]nativeDrama{
		"chengguo": {{ID: "chengguo:1", Source: "chengguo", Title: "同名剧", Episodes: 10}},
	}
	normal := aggregateByTitle(perSource, false)
	if len(normal) != 1 {
		t.Fatalf("normal view = %d", len(normal))
	}
	adult := aggregateByTitle(perSource, true)
	if len(adult) != 0 {
		t.Fatalf("adult view = %d, want 0", len(adult))
	}
}

func TestAggregateByTitleWithRealChengGuo(t *testing.T) {
	perSource := map[string][]nativeDrama{
		"chengguo": {{ID: "chengguo:dj-1", Source: "chengguo", Title: "新秘密教学", Episodes: 80}},
	}
	items := aggregateByTitle(perSource, false)
	if len(items) != 1 {
		t.Fatalf("chengguo items in normal view = %d, want 1", len(items))
	}
}

func TestSourceIsAdultChengGuo(t *testing.T) {
	if sourceIsAdult("chengguo") {
		t.Fatal("chengguo should not be adult")
	}
}

func TestNativeRankingBoardSupported(t *testing.T) {
	seen := map[string]bool{}
	for _, board := range rankingBoards {
		if !nativeRankingBoardSupported(board) {
			continue
		}
		if isRetiredSource(board.Source) {
			t.Errorf("board %s uses retired source %s", board.ID, board.Source)
		}
		seen[board.Source] = true
	}
	// S7 之后内置源清单为空，榜单只认仍在注册表里的源；
	// 这里断言「至少有一个可用榜单源，且没有指向已退役源」。
	if len(seen) == 0 {
		t.Fatal("没有任何可用榜单源")
	}
	for source := range seen {
		if !sourceInstalled(source) && !nativeSourceAvailable(source) {
			t.Fatalf("榜单源 %q 既无内置实现也无已装模块：%v", source, seen)
		}
	}
}

func TestAggregateByTitleDedupesWithinSource(t *testing.T) {
	perSource := map[string][]nativeDrama{
		"chengguo": {
			{ID: "cg:1", Source: "chengguo", Title: "同一部剧"},
			{ID: "cg:2", Source: "chengguo", Title: "同一部剧 第2季"},
		},
		"51duanju": {
			{ID: "51:1", Source: "51duanju", Title: "同一部剧"},
		},
	}
	items := aggregateByTitleTagged(perSource, false, true)
	if len(items) != 1 {
		t.Fatalf("应合并为一条：%+v", items)
	}
	if items[0].SourceCount != 2 {
		t.Fatalf("同源重复不应被算作多来源，实际 %d：%+v", items[0].SourceCount, items[0].Sources)
	}
}
