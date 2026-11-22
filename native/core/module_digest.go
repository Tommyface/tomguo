package core

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

// moduleDigestAllowMissing 控制「manifest 未写 mainSha256」时的策略。
// true  → 允许加载并记一条告警（灰度期：存量模块不必一次性补齐）
// false → 拒绝加载（存量补齐后切开，作为发布版本的默认值）
var moduleDigestAllowMissing = true

// moduleDigestHexLen 是 sha256 十六进制摘要的长度。
const moduleDigestHexLen = 64

// verifyModuleDigest 比对 manifest.mainSha256 与 main.js 实际字节。
// 返回 warning（缺摘要且允许缺省时非空）与 error（摘要缺失不被允许、格式非法或不一致）。
// 调用方必须在校验通过之后再编译脚本，否则等于执行未校验的代码。
func verifyModuleDigest(manifest moduleManifest, code []byte) (string, error) {
	want := strings.ToLower(strings.TrimSpace(manifest.MainSha256))
	if want == "" {
		if !moduleDigestAllowMissing {
			return "", moduleDigestError(manifest.ID, "缺少 mainSha256，模块来源无法验证")
		}
		return "模块未声明 mainSha256，已按兼容模式加载", nil
	}
	if len(want) != moduleDigestHexLen {
		return "", moduleDigestError(manifest.ID, "mainSha256 必须是 64 位十六进制")
	}
	if _, err := hex.DecodeString(want); err != nil {
		return "", moduleDigestError(manifest.ID, "mainSha256 不是合法的十六进制")
	}
	sum := sha256.Sum256(code)
	got := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		return "", moduleDigestError(manifest.ID, "main.js 与清单摘要不一致，可能已损坏或被篡改")
	}
	return "", nil
}

// moduleDigestError 统一错误前缀，界面可直接展示。
func moduleDigestError(id, reason string) error {
	if strings.TrimSpace(id) == "" {
		id = "未知模块"
	}
	return fmt.Errorf("模块 %s 内容校验失败：%s", id, reason)
}

// moduleDigestOf 计算摘要，供安装流程与工具复用。
func moduleDigestOf(code []byte) string {
	sum := sha256.Sum256(code)
	return hex.EncodeToString(sum[:])
}
