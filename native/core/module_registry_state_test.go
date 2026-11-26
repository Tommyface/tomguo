package core

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModuleRegistrySchema2StateFields(t *testing.T) {
	raw := `{"name":"test","api":2,"schema":2,"plugins":[
		{"id":"alpha","name":"Alpha","adult":false,"mainSha256":"` + strings.Repeat("a", 64) + `","zip":"https://example.com/alpha.zip","available":true,"state":"active"},
		{"id":"beta","name":"Beta","adult":false,"mainSha256":"` + strings.Repeat("b", 64) + `","zip":"https://example.com/beta.zip","available":false,"state":"retired","reason":"站点已关停","replacedBy":"alpha"},
		{"id":"gamma","name":"Gamma","adult":false,"mainSha256":"` + strings.Repeat("c", 64) + `","zip":"https://example.com/gamma.zip","state":"deprecated","reason":"即将停用"},
		{"id":"delta","name":"Delta","adult":false,"mainSha256":"` + strings.Repeat("d", 64) + `","zip":"https://example.com/delta.zip","state":"blocked","reason":"内容不合规"}
	]}`
	list, err := ParseModuleRegistry([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Entries) != 4 {
		t.Fatalf("条目数 %d，期望 4", len(list.Entries))
	}
	allowed := list.EntriesFiltered(false)
	if len(allowed) != 2 || allowed[0].ID != "alpha" || allowed[1].ID != "gamma" {
		t.Fatalf("可安装条目不符：%+v", allowed)
	}
	byID := map[string]moduleRegistryEntry{}
	for _, entry := range list.Entries {
		byID[entry.ID] = entry
	}
	if byID["beta"].StateLabel() != "站点已关停" || byID["gamma"].StateLabel() != "即将停用" {
		t.Fatalf("状态说明不符：beta=%q gamma=%q", byID["beta"].StateLabel(), byID["gamma"].StateLabel())
	}
	if !byID["alpha"].CanInstall() || byID["beta"].CanInstall() || !byID["gamma"].CanInstall() || byID["delta"].CanInstall() {
		t.Fatal("CanInstall 判定不符")
	}
}

func TestModuleRegistryLegacyWithoutState(t *testing.T) {
	raw := `{"name":"legacy","api":1,"plugins":[{"id":"old","name":"Old","adult":false,"mainSha256":"` + strings.Repeat("e", 64) + `","zip":"https://example.com/old.zip"}]}`
	list, err := ParseModuleRegistry([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got := list.EntriesFiltered(false); len(got) != 1 || !got[0].CanInstall() {
		t.Fatalf("旧清单条目应保持可安装：%+v", got)
	}
}

func TestModuleRegistryRejectsBadState(t *testing.T) {
	cases := []string{
		`{"api":2,"plugins":[{"id":"x","name":"X","adult":false,"mainSha256":"` + strings.Repeat("a", 64) + `","zip":"https://example.com/x.zip","state":"unknown"}]}`,
		`{"api":2,"plugins":[{"id":"x","name":"X","adult":false,"mainSha256":"` + strings.Repeat("a", 64) + `","zip":"https://example.com/x.zip","state":"retired"}]}`,
		`{"api":2,"plugins":[{"id":"x","name":"X","adult":false,"mainSha256":"` + strings.Repeat("a", 64) + `","zip":"https://example.com/x.zip","available":false}]}`,
	}
	for index, raw := range cases {
		if _, err := ParseModuleRegistry([]byte(raw)); err == nil {
			t.Errorf("用例 %d 应该被拒绝", index)
		}
	}
}

func TestModuleRegistryStateJSONCompat(t *testing.T) {
	entry := moduleRegistryEntry{}
	if err := json.Unmarshal([]byte(`{"id":"x","available":false,"state":"blocked"}`), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Available == nil || *entry.Available || entry.State != "blocked" {
		t.Fatalf("字段解析不符：%+v", entry)
	}
}
