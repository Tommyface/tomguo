package core

import (
	"fmt"
	"regexp"
	"strings"

	"duanjuapp/native/source"
)

// 模块导出有两种写法：exports.xxx = fn 与 module.exports = {xxx: fn}。
// 静态扫描同时覆盖两种，避免导入时执行未校验代码。
var directExportPattern = regexp.MustCompile(`(?:^|[^\w.$])(?:module\.)?exports\.([A-Za-z_$][\w$]*)\s*=`)

// exportedActions 扫描源码里导出的动作名。对象字面量用括号配对定位完整块，
// 不能用非贪婪正则——它会在第一个内部对象的 } 处截断。
func exportedActions(code []byte) map[string]bool {
	found := map[string]bool{}
	text := string(code)
	for _, match := range directExportPattern.FindAllStringSubmatch(text, -1) {
		found[match[1]] = true
	}
	for _, block := range objectExportBlocks(text) {
		for _, key := range objectLiteralKeys(block) {
			found[key] = true
		}
	}
	return found
}

// objectExportBlocks 找所有 module.exports = { ... } 的完整块，按大括号配对。
func objectExportBlocks(text string) []string {
	marker := "module.exports"
	blocks := []string{}
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], marker)
		if index < 0 {
			break
		}
		start := offset + index + len(marker)
		brace := strings.IndexAny(text[start:], "{")
		if brace < 0 {
			break
		}
		open := start + brace
		depth, end := 0, -1
		for i := open; i < len(text); i++ {
			switch text[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = i
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			break
		}
		blocks = append(blocks, text[open+1:end])
		offset = end + 1
	}
	return blocks
}

// objectLiteralKeys 取对象顶层键：只在 depth==1 且键后紧跟冒号时认定。
func objectLiteralKeys(block string) []string {
	keys := []string{}
	depth := 0
	i := 0
	for i < len(block) {
		char := block[i]
		switch char {
		case '{', '[':
			depth++
			i++
		case '}', ']':
			depth--
			i++
		case '/':
			if i+1 < len(block) && block[i+1] == '/' {
				for i < len(block) && block[i] != '\n' {
					i++
				}
				continue
			}
			i++
		default:
			if depth != 0 || !isIdentStart(char) {
				i++
				continue
			}
			start := i
			for i < len(block) && isIdentPart(block[i]) {
				i++
			}
			key := block[start:i]
			j := i
			for j < len(block) && (block[j] == ' ' || block[j] == '\t' || block[j] == '\n' || block[j] == '\r') {
				j++
			}
			if j < len(block) && block[j] == ':' {
				keys = append(keys, key)
			}
		}
	}
	return keys
}

func isIdentStart(char byte) bool {
	return char == '_' || char == '$' || (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z')
}

func isIdentPart(char byte) bool {
	return isIdentStart(char) || (char >= '0' && char <= '9')
}

// checkModuleContract 在编译前校验模块导出的五个动作齐全。
func checkModuleContract(id string, code []byte, capabilities []string) error {
	found := exportedActions(code)
	missing := []string{}
	for _, action := range source.RequiredExports(capabilities) {
		if !found[action] {
			missing = append(missing, action)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("模块 %s 缺少必需导出：%s", id, strings.Join(missing, "、"))
}
