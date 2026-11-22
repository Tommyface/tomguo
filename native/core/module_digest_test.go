package core

import "testing"

func TestVerifyModuleDigest(t *testing.T) {
	code := []byte("module.exports={};")
	sum := moduleDigestOf(code)

	if warning, err := verifyModuleDigest(moduleManifest{ID: "demo", MainSha256: sum}, code); err != nil || warning != "" {
		t.Fatalf("匹配摘要应通过，得到 warning=%q err=%v", warning, err)
	}
	if _, err := verifyModuleDigest(moduleManifest{ID: "demo", MainSha256: sum}, []byte("module.exports=1;")); err == nil {
		t.Fatal("摘要不匹配应失败")
	}
	if _, err := verifyModuleDigest(moduleManifest{ID: "demo", MainSha256: "abc"}, code); err == nil {
		t.Fatal("长度非法应失败")
	}
	if _, err := verifyModuleDigest(moduleManifest{ID: "demo", MainSha256: ""}, code); err != nil {
		t.Fatal("兼容模式不应失败")
	}
	moduleDigestAllowMissing = false
	if _, err := verifyModuleDigest(moduleManifest{ID: "demo", MainSha256: ""}, code); err == nil {
		t.Fatal("严格模式缺摘要应失败")
	}
	moduleDigestAllowMissing = true
}
