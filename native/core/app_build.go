package core

import (
	"errors"
	"strings"
)

var errNativeBuildSource = errors.New("当前版本不包含此站源")

// sourceInstalled 判断站源当前是否有可执行实现：已安装模块或仍存在的内置源。
// S7 之后内置源清单为空，未安装的 id 既不能取目录，也不该进入预热与封面补全队列。
func sourceInstalled(source string) bool {
	normalized := canonicalProviderSource(strings.ToLower(strings.TrimSpace(source)))
	if normalized == "" {
		return false
	}
	if moduleSource(normalized) != nil {
		return true
	}
	_, ok := lookupSourceSpec(normalized)
	return ok
}

func nativeSourceAvailable(source string) bool {
	normalized := canonicalProviderSource(source)
	// S7：已安装的 JS 模块接管同名内置源时，即使对应的 Go provider 已删除，
	// 该源仍然是可访问的；授权白名单必须放行，否则模块路径会在入口被拦掉。
	if moduleTakesOver(normalized) {
		return true
	}
	return isHuangguoProviderSource(normalized)
}

func nativeDramaAvailable(drama nativeDrama) bool {
	source, _, valid := splitProviderDramaID(drama.ID)
	return valid && nativeSourceAvailable(source) &&
		(drama.Source == "" || canonicalProviderSource(drama.Source) == source)
}

func nativeChapterAvailable(drama nativeDrama, chapter Chapter) bool {
	if !nativeDramaAvailable(drama) {
		return false
	}
	source, _, _ := splitProviderDramaID(drama.ID)
	if chapter.Source != "" && canonicalProviderSource(chapter.Source) != source {
		return false
	}
	if chapterSource, _, valid := splitProviderDramaID(chapter.ID); valid && chapterSource != source {
		return false
	}
	return true
}

func nativeDownloadAvailable(job nativeDownloadJob) bool {
	return nativeChapterAvailable(job.Drama, job.Chapter)
}

func nativeAuthorizeInput(input nativeInput) error {
	switch input.Action {
	case "recommendations", "cachedRecommendations", "suggestions", "danmaku":
		if !nativeSourceAvailable(sourceHongguo) {
			return errNativeBuildSource
		}
	case "rankings":
		board, found := findRankingBoard(input.Board)
		if found {
			if !nativeSourceAvailable(board.Source) {
				return errNativeBuildSource
			}
			return nil
		}
		// 模块榜单板由分类动态生成，ID 形如 <source>-<category>，不在这张内置表里。
		source, ok := moduleRankingBoardSource(input.Board)
		if !ok || !nativeSourceAvailable(source) {
			return errNativeBuildSource
		}
	case "catalog", "cached", "categories", "sourceStatus", "sourceJob", "cancelSourceJob":
		if !nativeSourceAvailable(input.Source) {
			return errNativeBuildSource
		}
		if input.Action == "sourceJob" && input.Drama.ID != "" &&
			(!nativeDramaAvailable(input.Drama) || sourceFromDramaID(input.Drama.ID) != canonicalProviderSource(input.Source)) {
			return errNativeBuildSource
		}
	case "modules", "removeModule":
		return nil
	case "installModule":
		if strings.TrimSpace(input.ModulePath) == "" {
			return errNativeBuildSource
		}
		return nil
	case "crossSource":
		if strings.TrimSpace(input.Query) == "" && strings.TrimSpace(input.Drama.Title) == "" {
			return errNativeBuildSource
		}
	case "cover", "prepareCover", "detail", "metadata", "resolve", "preload", "prepareHandoff", "enqueueDownloads", "localPlayback":
		if !nativeDramaAvailable(input.Drama) {
			return errNativeBuildSource
		}
		if (input.Action == "resolve" || input.Action == "preload" || input.Action == "prepareHandoff") && !nativeChapterAvailable(input.Drama, input.Chapter) {
			return errNativeBuildSource
		}
	}
	return nil
}
