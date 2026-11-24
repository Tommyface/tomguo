package core

import (
	"context"
	"testing"
)

func TestWarmCatalogsSkipsRetiredAndFresh(t *testing.T) {
	engine := &nativeEngine{
		catalogs:      map[string][]nativeDrama{},
		catalogStates: map[string]nativeCatalogState{},
		sourceProbes:  map[string]nativeSourceProbe{},
	}
	results := engine.warmCatalogs(context.Background(), []string{sourceHuangguoAI, sourceYeguo}, false)
	if len(results) != 0 {
		t.Fatalf("retired sources must not warm, got %#v", results)
	}
}

func TestWarmCatalogsDedupesAndReportsSources(t *testing.T) {
	engine := &nativeEngine{
		catalogs:      map[string][]nativeDrama{},
		catalogStates: map[string]nativeCatalogState{},
		sourceProbes:  map[string]nativeSourceProbe{},
	}
	results := engine.warmCatalogs(context.Background(), []string{"HG", "hg", sourceHuangguoAI}, false)
	if len(results) != 1 || results[0].Source != "hg" {
		t.Fatalf("duplicate sources must collapse: %#v", results)
	}
	if results[0].ElapsedMS < 0 {
		t.Fatalf("elapsed must be non-negative: %#v", results[0])
	}
}
