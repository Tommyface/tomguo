package core

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

func TestNormalizeCustomConfigMaccms(t *testing.T) {
	cfg := customSourceConfig{
		Name:              "我的短剧站",
		BaseURL:           "https://example.com/",
		Type:              "maccms",
		DetailPathPattern: `^/voddetail/([0-9]+)\.html$`,
	}
	got, err := normalizeCustomConfig(cfg)
	if err != nil {
		t.Fatalf("规范化失败: %v", err)
	}
	if !isCustomSource(got.ID) {
		t.Fatalf("应自动加 custom_ 前缀，实际 %q", got.ID)
	}
	if got.BaseURL != "https://example.com" {
		t.Fatalf("应去掉尾部斜杠，实际 %q", got.BaseURL)
	}
	if got.DetailURLTemplate != "{base}{id}" {
		t.Fatalf("应填默认详情模板，实际 %q", got.DetailURLTemplate)
	}
}

func TestNormalizeCustomConfigRejectsInvalid(t *testing.T) {
	cases := []customSourceConfig{
		{Name: "", BaseURL: "https://a.com", Type: "maccms", DetailPathPattern: `^/d/(\d+)$`},
		{Name: "x", BaseURL: "not-a-url", Type: "maccms", DetailPathPattern: `^/d/(\d+)$`},
		{Name: "x", BaseURL: "https://a.com", Type: "maccms", DetailPathPattern: `^/d/(\d+`}, // 坏正则
		{Name: "x", BaseURL: "https://a.com", Type: "aes"},                                   // 缺 key
		{Name: "x", BaseURL: "https://a.com", Type: "unknown"},
	}
	for i, cfg := range cases {
		if _, err := normalizeCustomConfig(cfg); err == nil {
			t.Fatalf("用例 %d 应拒绝无效配置: %+v", i, cfg)
		}
	}
}

func TestCustomRegistryReplaceAndLookup(t *testing.T) {
	r := newCustomRegistry()
	configs := []customSourceConfig{
		{ID: "custom_a", Name: "A站", BaseURL: "https://a.com", Type: "maccms", DetailPathPattern: `^/d/(\d+)$`},
		{ID: "custom_b", Name: "B站", BaseURL: "https://b.com", Type: "aes", AESKey: "1234567890abcdef"},
	}
	count, err := r.replace(configs)
	if err != nil {
		t.Fatalf("replace 失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("应有 2 个源，实际 %d", count)
	}
	a, ok := r.get("custom_a")
	if !ok || a.Name != "A站" {
		t.Fatalf("查不到 custom_a: %+v", a)
	}
	if got := r.sourceForHost("a.com"); got != "custom_a" {
		t.Fatalf("host 反查应为 custom_a，实际 %q", got)
	}
	// 全量替换后旧的应消失。
	if _, err := r.replace([]customSourceConfig{
		{ID: "custom_c", Name: "C站", BaseURL: "https://c.com", Type: "maccms", DetailPathPattern: `^/d/(\d+)$`},
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.get("custom_a"); ok {
		t.Fatal("全量替换后旧源 custom_a 应移除")
	}
}

func TestMaccmsProfileFromCustomConfig(t *testing.T) {
	cfg, err := normalizeCustomConfig(customSourceConfig{
		ID:                "custom_test",
		Name:              "测试站",
		BaseURL:           "https://example.com",
		Type:              "maccms",
		DetailPathPattern: `^/voddetail/([0-9]+)\.html$`,
		PlayPathPattern:   `^/vodplay/[0-9]+-[0-9]+-[0-9]+\.html$`,
		DetailURLTemplate: `{base}/voddetail/{id}.html`,
		ListURLTemplate:   `{base}/vodtype/1-{page}.html`,
		SearchURLTemplate: `{base}/search?wd={query}&page={page}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	profile, err := cfg.maccmsProfile()
	if err != nil {
		t.Fatal(err)
	}
	if got := maccmsDetailID("https://example.com/voddetail/123.html", profile); got != "123" {
		t.Fatalf("详情 ID 应为 123，实际 %q", got)
	}
	if got := profile.DetailURL("https://example.com", "456"); got != "https://example.com/voddetail/456.html" {
		t.Fatalf("详情 URL 错误: %q", got)
	}
	if got := profile.ListURL("https://example.com", 3); got != "https://example.com/vodtype/1-3.html" {
		t.Fatalf("目录 URL 错误: %q", got)
	}
	if got := profile.SearchURL("https://example.com", "剧", 2); got != "https://example.com/search?wd=%E5%89%A7&page=2" {
		t.Fatalf("搜索 URL 错误: %q", got)
	}
}

func TestDecodeCustomSourcesBothShapes(t *testing.T) {
	// 裸数组
	arrRaw, _ := json.Marshal([]customSourceConfig{
		{Name: "A", BaseURL: "https://a.com", Type: "maccms", DetailPathPattern: `^/d/(\d+)$`},
	})
	got, err := decodeCustomSources(arrRaw)
	if err != nil || len(got) != 1 {
		t.Fatalf("裸数组解析失败: %v, %+v", err, got)
	}
	// wrapper
	got, err = decodeCustomSources(json.RawMessage(`{"sources":[{"name":"B","baseUrl":"https://b.com","type":"aes","aesKey":"0123456789abcdef"}]}`))
	if err != nil || len(got) != 1 || got[0].Name != "B" {
		t.Fatalf("wrapper 解析失败: %v, %+v", err, got)
	}
}

func TestAesDecryptPayload(t *testing.T) {
	cfg := customSourceConfig{AESKey: "1234567890123456", AESIV: "6543210987654321"}
	plain := []byte(`{"list":[{"id":"1","title":"测试"}]}`)
	key, _ := decodeAESKey(cfg.AESKey, false)
	iv, _ := decodeAESKey(cfg.AESIV, false)
	enc, err := aesCBCEncrypt(plain, key, iv)
	if err != nil {
		t.Fatal(err)
	}
	dataStr := base64.StdEncoding.EncodeToString(enc)
	out, err := aesDecryptPayload([]byte(dataStr), cfg)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	json.Unmarshal(plain, &a)
	json.Unmarshal(out, &b)
	if !jsonEqual(a, b) {
		t.Fatalf("解密结果不符: %s", out)
	}
}

func TestDynamicProfileIntegration(t *testing.T) {
	d := &Downloader{customSources: newCustomRegistry()}
	cfg, err := normalizeCustomConfig(customSourceConfig{
		ID: "custom_x", Name: "X", BaseURL: "https://x.com", Type: "maccms",
		DetailPathPattern: `^/d/(\d+)\.html$`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.customSources.replace([]customSourceConfig{cfg}); err != nil {
		t.Fatal(err)
	}
	profile, found := d.maccmsProfileForDynamic("custom_x")
	if !found || profile.Name != "X" {
		t.Fatalf("动态 profile 查找失败: %+v", profile)
	}
	if !isHuangguoProviderSource("custom_x") {
		t.Fatal("动态源应被白名单认可")
	}
	if _, found := d.maccmsProfileForDynamic("nonexistent"); found {
		t.Fatal("不存在的源不应找到 profile")
	}
}

func jsonEqual(a, b any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}
