package core

import (
	"strings"
)

// moduleCoverPinned 判断封面域名是否被某个已装模块声明为图床。
// 声明过的域名直接走 DoH + 钉 IP：这些图床在国内网络里直连必然超时，
// 先直连只会白等一轮。未声明的域名仍按「先直连、失败再回退」处理。
func moduleCoverPinned(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, manifest := range ModuleManifests() {
		for _, declared := range manifest.CoverHosts {
			declared = strings.ToLower(strings.TrimSpace(declared))
			if declared == "" {
				continue
			}
			if host == declared || strings.HasSuffix(host, "."+declared) {
				return true
			}
		}
	}
	return false
}
