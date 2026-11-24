package core

import "testing"

func TestModuleCoverPinnedMatchesDeclaredHosts(t *testing.T) {
	saved := moduleRegistry.modules
	defer func() { moduleRegistry.modules = saved }()
	moduleRegistry.modules = map[string]*jsModule{
		"demo": {manifest: moduleManifest{ID: "demo", CoverHosts: []string{"madou.club", "img.example.test"}}},
	}
	for _, host := range []string{"madou.club", "MADOU.CLUB", "cdn.madou.club", "img.example.test"} {
		if !moduleCoverPinned(host) {
			t.Fatalf("host %q should be pinned", host)
		}
	}
	for _, host := range []string{"", "notmadou.club", "example.test", "othermadou.club"} {
		if moduleCoverPinned(host) {
			t.Fatalf("host %q must not be pinned", host)
		}
	}
}

func TestModuleCoverPinnedEmptyWhenNothingDeclared(t *testing.T) {
	saved := moduleRegistry.modules
	defer func() { moduleRegistry.modules = saved }()
	moduleRegistry.modules = map[string]*jsModule{}
	if moduleCoverPinned("madou.club") {
		t.Fatal("no declaration means no pinning")
	}
}
