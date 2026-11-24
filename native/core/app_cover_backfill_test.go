package core

import (
	"testing"
	"time"
)

func TestPendingCoverBackfillLimitsAndSkipsExisting(t *testing.T) {
	engine := &nativeEngine{catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}}
	items := []nativeDrama{{ID: "a:1"}, {ID: "a:2", Cover: "https://img/1.jpg"}, {ID: "a:3"}}
	picked := engine.pendingCoverBackfill(items)
	if len(picked) != 2 || picked[0] != "a:1" || picked[1] != "a:3" {
		t.Fatalf("picked = %v", picked)
	}
	if again := engine.pendingCoverBackfill(items); len(again) != 0 {
		t.Fatalf("repeated pick must be skipped, got %v", again)
	}
	engine.coverBackfill["a:1"] = time.Now().Add(-nativeCoverBackfillTTL - time.Minute)
	if retry := engine.pendingCoverBackfill(items); len(retry) != 1 || retry[0] != "a:1" {
		t.Fatalf("expired attempt should retry, got %v", retry)
	}
}

func TestPendingCoverBackfillRespectsLimit(t *testing.T) {
	engine := &nativeEngine{catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}}
	items := make([]nativeDrama, 0, 20)
	for index := 0; index < 20; index++ {
		items = append(items, nativeDrama{ID: "a:" + string(rune('a'+index))})
	}
	if picked := engine.pendingCoverBackfill(items); len(picked) != nativeCoverBackfillLimit {
		t.Fatalf("picked %d, want %d", len(picked), nativeCoverBackfillLimit)
	}
}

func TestBackfillCoversSkipsRetiredSource(t *testing.T) {
	engine := &nativeEngine{catalogs: map[string][]nativeDrama{}, catalogStates: map[string]nativeCatalogState{}}
	engine.backfillCovers(sourceHuangguoAI, []nativeDrama{{ID: sourceHuangguoAI + ":1"}})
	if len(engine.coverBackfill) != 0 {
		t.Fatalf("retired source must not backfill: %v", engine.coverBackfill)
	}
}
