package core

import (
	"strings"
	"testing"
)

var sampleRegistry = `{
  "name": "测试清单",
  "api": 1,
  "plugins": [
    {"id":"demo","name":"演示","version":"1.0.0","adult":false,"mainSha256":"` + demoSum + `","zip":"https://x/demo.zip","capabilities":["search"]},
    {"id":"adultone","name":"成人","version":"1.0.0","adult":true,"mainSha256":"` + demoSum + `","zip":"https://x/a.zip"}
  ]
}`

var demoCode = []byte("module.exports={};")
var demoSum = moduleDigestOf(demoCode)

func TestParseModuleRegistry(t *testing.T) {
	list, err := ParseModuleRegistry([]byte(sampleRegistry))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(list.Entries) != 2 {
		t.Fatalf("条目数 %d", len(list.Entries))
	}
	if got := list.EntriesFiltered(false); len(got) != 1 || got[0].ID != "demo" {
		t.Fatalf("非成人过滤错误：%v", got)
	}
	if got := list.EntriesFiltered(true); len(got) != 2 {
		t.Fatalf("包含成人应为 2 条，得到 %d", len(got))
	}
	if entry, ok := list.FindEntry("ADULTONE"); !ok || entry.ID != "adultone" {
		t.Fatalf("FindEntry 大小写归一失败：%v %v", entry, ok)
	}
}

func TestParseModuleRegistryRejects(t *testing.T) {
	cases := map[string]string{
		"缺 adult": `{"api":1,"plugins":[{"id":"a","name":"n","mainSha256":"` + demoSum + `","zip":"z"}]}`,
		"摘要过短":    `{"api":1,"plugins":[{"id":"a","name":"n","adult":false,"mainSha256":"abc","zip":"z"}]}`,
		"重复 id":   `{"api":1,"plugins":[{"id":"a","name":"n","adult":false,"mainSha256":"` + demoSum + `","zip":"z"},{"id":"a","name":"n2","adult":false,"mainSha256":"` + demoSum + `","zip":"z2"}]}`,
		"清单版本过新":  `{"api":9,"plugins":[{"id":"a","name":"n","adult":false,"mainSha256":"` + demoSum + `","zip":"z"}]}`,
		"缺下载地址":   `{"api":1,"plugins":[{"id":"a","name":"n","adult":false,"mainSha256":"` + demoSum + `"}]}`,
		"空清单":     `{"api":1,"plugins":[]}`,
	}
	for name, body := range cases {
		if _, err := ParseModuleRegistry([]byte(body)); err == nil {
			t.Fatalf("%s 应当被拒绝", name)
		}
	}
}

func TestVerifyEntryCode(t *testing.T) {
	entry := moduleRegistryEntry{ID: "demo", MainSha256: demoSum}
	if err := VerifyEntryCode(entry, demoCode); err != nil {
		t.Fatalf("匹配应通过：%v", err)
	}
	if err := VerifyEntryCode(entry, []byte("tampered")); err == nil || !strings.Contains(err.Error(), "校验失败") {
		t.Fatalf("篡改应失败，得到 %v", err)
	}
	if err := VerifyEntryCode(moduleRegistryEntry{ID: "demo"}, demoCode); err == nil {
		t.Fatal("空摘要应失败")
	}
}
