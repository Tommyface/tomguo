package core

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// htmlUnescapeDeep 反复解 HTML 实体与数字引用，最多 3 轮。
// 站点会把标题做多层转义（&amp;lt; 之类），一轮不够。
// 原本实现在 provider_guipian.go 里，站源迁走后被多个源共用，移到中立文件。
func htmlUnescapeDeep(text string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", "\"",
		"&apos;", "'", "&#39;", "'", "&nbsp;", " ",
	)
	hexPattern := regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	decPattern := regexp.MustCompile(`&#(\d+);`)
	for i := 0; i < 3; i++ {
		next := replacer.Replace(text)
		next = hexPattern.ReplaceAllStringFunc(next, func(m string) string {
			if code, err := strconv.ParseInt(m[3:len(m)-1], 16, 32); err == nil {
				return string(rune(code))
			}
			return m
		})
		next = decPattern.ReplaceAllStringFunc(next, func(m string) string {
			if code, err := strconv.Atoi(m[2 : len(m)-1]); err == nil {
				return string(rune(code))
			}
			return m
		})
		if next == text {
			break
		}
		text = next
	}
	return text
}

// firstAnyValue 返回 row 中第一个存在且非空字符串值的字段，供多个 JSON 源共用。
func firstAnyValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := row[key]; ok && nativeText(value) != "" {
			return value
		}
	}
	return nil
}

// isListresPlaylist 判断媒体地址是不是 E 站播放器用的 listres 清单（.webp 后缀但交给 HLS 加载）。
// 站源 Go 实现删除后，provider_media.go 仍需要这个判断，故移到中立文件。
func isListresPlaylist(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && strings.Contains(strings.ToLower(parsed.Path), "/index/listres/")
}
