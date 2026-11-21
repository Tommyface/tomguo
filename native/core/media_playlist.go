package core

import (
	"math"

	"strconv"
	"strings"
	"time"
)

// playlistStats 汇总一条 m3u8 播放列表的时长与分片数。
// 只做只读解析：既给调用方提供总时长，也提供分片数量，供完整性判断使用。
type playlistStats struct {
	Duration  time.Duration
	Segments  int
	ValidM3U8 bool
}

func inspectPlaylist(raw string) playlistStats {
	trimmed := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
	stats := playlistStats{ValidM3U8: strings.HasPrefix(trimmed, "#EXTM3U")}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "#EXTINF:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "#EXTINF:"))
		if comma := strings.IndexByte(value, ','); comma >= 0 {
			value = value[:comma]
		}
		seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || seconds < 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
			continue
		}
		stats.Duration += time.Duration(seconds * float64(time.Second))
		stats.Segments++
	}
	return stats
}

func m3u8Duration(raw string) time.Duration {
	return inspectPlaylist(raw).Duration
}

// previewDurationLimit 是判断"疑似预览/占位片段"的时长阈值。
// 短剧单集通常在数十秒以上；低于此值且分片极少的列表，多为站源下发的试看占位内容。
const previewDurationLimit = 15 * time.Second

// looksLikePreviewFragment 判断播放列表是否疑似试看/占位片段。
// 这是只读的工程判断，仅用于诊断提示，不拦截播放、不改变任何现有行为。
// 判定条件保守：必须是合法 m3u8、有分片、且总时长明显偏短，避免误伤正常短视频。
func looksLikePreviewFragment(stats playlistStats) bool {
	return stats.ValidM3U8 && stats.Segments > 0 && stats.Duration > 0 && stats.Duration < previewDurationLimit
}
