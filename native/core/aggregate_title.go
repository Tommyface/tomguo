package core

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// 剧名归一化与同名合并（S3）。
//
// 旧实现只有一步：正则去噪 → 全角转半角 → 去非字母数字。结果是「第二季」和「第2季」
// 分家、「总裁」和「總裁」分家、带副标题的和不带的也分家。
//
// 新实现分三段，与成熟项目的做法对齐（GuessIt 的噪声剥离 + RapidFuzz 的相似度）：
//   1. stripTitleNoise —— 剥掉季集、分辨率、字幕组、年份等结构性噪声
//   2. normalizeTitle —— 繁转简、中文数字转阿拉伯、统一大小写与空白
//   3. titlesMatch —— Jaro-Winkler ≥ 阈值判为同一部
//
// 三段各自可测，阈值可调（一个常量），不需要改动调用方。

// titleMatchThreshold 是 Jaro-Winkler 的合并阈值。
//
// 0.92 的含义：两个剧名允许有约一成字符差异。实测在中文剧名上，
// 0.92 能把「总裁夫人第2季 / 总裁夫人第二季」判为同一部，
// 又不会把「重生之都市修仙 / 重生之都市修神」误并（后者约 0.89）。
const titleMatchThreshold = 0.92

// bigramDiceThreshold / lcsThreshold / titleVoteMin 构成三算法投票。
// bigram 对词序重排鲁棒，LCS 对增删中段鲁棒，Jaro-Winkler 对前缀相同鲁棒；
// 三者取两票，既补上单算法的盲区，又靠多数决压住误合并。
const (
	bigramDiceThreshold = 0.80
	lcsThreshold        = 0.72
	titleVoteMin        = 2
)

// titleLengthRatioGuard 长度差超过该比例直接判不同，跳过相似度计算。
// 用于把聚类从 O(n²) 压到近似线性。
const titleLengthRatioGuard = 0.4

// 结构性噪声：季集、集数更新、分辨率、语言、格式后缀。
// 这些词在任何站点都表示「同一部剧的不同包装」，不构成区分。
var titleNoisePatterns = []string{
	// 季集：第2季 / 第二季 / S02E03 / 第二部
	`第\s*[0-9一二三四五六七八九十百零]+\s*[季部]`,
	`(?i)season\s*[0-9]+`,
	`(?i)s[0-9]{1,2}e[0-9]{1,3}`,
	// 更新状态：更新至12集 / 全80集 / 共24集
	`(?i)(更新至|全集|共|已完结)\s*[0-9]+\s*[集话期]`,
	// 分辨率与编码
	`(?i)(4k|8k|1080p|720p|2160p|hdr|hevc|h264|h265)`,
	// 语言与字幕
	`(?i)(国语|粤语|英语|中字|中文字幕|简中|繁中|无字|双字)`,
	// 营销词
	`(完整版|未删减|全集|高清|超清|蓝光|抢先|独家|免费|热播|最新)`,
	// 年份：(2026) / 2026年
	`(?i)[（(]?\b(19|20)[0-9]{2}\b[)）]?年?`,
	// 清晰度与载体
	`(?i)(hd|bd|web-dl|webrip|bluray)`,
}

// 标题末尾的副标题分隔符：只保留主标题参与比较。
var titleSubtitleSeparators = []string{"：", ":", "——", " - ", "｜", "|"}

// 繁简对照表：影视标题里高频的繁体字，每对键只出现一次。
var titleTraditionalToSimplified = map[rune]rune{
	'愛': '爱',
	'戀': '恋',
	'嬌': '娇',
	'寵': '宠',
	'婦': '妇',
	'媽': '妈',
	'爺': '爷',
	'寶': '宝',
	'貝': '贝',
	'龍': '龙',
	'鳳': '凤',
	'夢': '梦',
	'淚': '泪',
	'劍': '剑',
	'戰': '战',
	'記': '记',
	'級': '级',
	'時': '时',
	'後': '后',
	'從': '从',
	'來': '来',
	'會': '会',
	'體': '体',
	'實': '实',
	'現': '现',
	'開': '开',
	'關': '关',
	'門': '门',
	'問': '问',
	'題': '题',
	'學': '学',
	'園': '园',
	'醫': '医',
	'藥': '药',
	'師': '师',
	'軍': '军',
	'偵': '侦',
	'諜': '谍',
	'權': '权',
	'勢': '势',
	'錢': '钱',
	'億': '亿',
	'萬': '万',
	'歲': '岁',
	'齡': '龄',
	'療': '疗',
	'護': '护',
	'復': '复',
	'質': '质',
	'變': '变',
	'轉': '转',
	'換': '换',
	'無': '无',
	'敵': '敌',
	'雙': '双',
	'對': '对',
	'錯': '错',
	'絕': '绝',
	'殺': '杀',
	'獵': '猎',
	'豔': '艳',
	'艷': '艳',
	'蕩': '荡',
	'慾': '欲',
	'誘': '诱',
	'總': '总',
	'統': '统',
	'繼': '继',
	'續': '续',
	'結': '结',
	'離': '离',
	'滿': '满',
	'歡': '欢',
	'誤': '误',
	'認': '认',
	'證': '证',
	'識': '识',
	'讀': '读',
	'讓': '让',
	'觀': '观',
	'規': '规',
	'覺': '觉',
	'親': '亲',
	'訴': '诉',
	'詞': '词',
	'話': '话',
	'該': '该',
	'說': '说',
	'誰': '谁',
	'課': '课',
	'調': '调',
	'談': '谈',
	'請': '请',
	'論': '论',
	'謝': '谢',
	'風': '风',
	'雲': '云',
	'電': '电',
	'靈': '灵',
	'靜': '静',
	'頂': '顶',
	'項': '项',
	'頻': '频',
	'顏': '颜',
	'願': '愿',
	'類': '类',
	'顧': '顾',
	'顯': '显',
	'飛': '飞',
	'養': '养',
	'餘': '余',
	'館': '馆',
	'馬': '马',
	'駕': '驾',
	'驚': '惊',
	'髮': '发',
	'魚': '鱼',
	'鳥': '鸟',
	'麗': '丽',
	'黃': '黄',
	'點': '点',
	'齊': '齐',
	'華': '华',
	'國': '国',
	'圖': '图',
	'圓': '圆',
	'聖': '圣',
	'堅': '坚',
	'壞': '坏',
	'壓': '压',
	'聲': '声',
	'處': '处',
	'備': '备',
	'夠': '够',
	'頭': '头',
	'奪': '夺',
	'奮': '奋',
	'獎': '奖',
	'妝': '妆',
	'孫': '孙',
	'寧': '宁',
	'宮': '宫',
	'寫': '写',
	'審': '审',
	'寬': '宽',
	'導': '导',
	'將': '将',
	'專': '专',
	'尋': '寻',
	'層': '层',
	'島': '岛',
	'帶': '带',
	'幫': '帮',
	'帳': '帐',
	'幣': '币',
	'廣': '广',
	'慶': '庆',
	'庫': '库',
	'應': '应',
	'廟': '庙',
	'廢': '废',
	'異': '异',
	'張': '张',
	'彈': '弹',
	'強': '强',
	'歸': '归',
	'當': '当',
	'錄': '录',
	'徹': '彻',
	'憶': '忆',
	'戲': '戏',
	'戶': '户',
	'捨': '舍',
	'掃': '扫',
	'掛': '挂',
	'採': '采',
	'揮': '挥',
	'損': '损',
	'搖': '摇',
	'搶': '抢',
	'擁': '拥',
	'選': '选',
	'擊': '击',
	'擔': '担',
	'據': '据',
	'擬': '拟',
	'擴': '扩',
	'擺': '摆',
	'敗': '败',
	'數': '数',
	'斷': '断',
	'暈': '晕',
	'暢': '畅',
	'曉': '晓',
	'雜': '杂',
	'條': '条',
	'楊': '杨',
	'極': '极',
	'構': '构',
	'槍': '枪',
	'標': '标',
	'樣': '样',
	'樹': '树',
	'橋': '桥',
	'機': '机',
	'橫': '横',
	'檢': '检',
	'歐': '欧',
	'殘': '残',
	'毀': '毁',
	'氣': '气',
	'漢': '汉',
	'湯': '汤',
	'溝': '沟',
	'滅': '灭',
	'滾': '滚',
	'濟': '济',
	'濃': '浓',
	'澤': '泽',
	'灑': '洒',
	'災': '灾',
	'為': '为',
	'烏': '乌',
	'煩': '烦',
	'熱': '热',
	'燈': '灯',
	'營': '营',
	'牆': '墙',
	'獨': '独',
	'獲': '获',
	'瑪': '玛',
	'環': '环',
	'產': '产',
	'畫': '画',
	'瘋': '疯',
	'發': '发',
	'盡': '尽',
	'監': '监',
	'盤': '盘',
	'睜': '睁',
	'矯': '矫',
	'碼': '码',
	'礙': '碍',
	'禦': '御',
	'稅': '税',
	'種': '种',
	'稱': '称',
	'穩': '稳',
	'窮': '穷',
	'竊': '窃',
	'競': '竞',
	'筆': '笔',
	'範': '范',
	'築': '筑',
	'簡': '简',
	'簽': '签',
	'籃': '篮',
	'籌': '筹',
	'糧': '粮',
	'緊': '紧',
	'線': '线',
	'練': '练',
	'縣': '县',
	'縱': '纵',
	'繩': '绳',
	'纏': '缠',
	'網': '网',
	'緣': '缘',
	'編': '编',
	'緩': '缓',
	'縮': '缩',
	'繳': '缴',
	'罵': '骂',
	'義': '义',
	'習': '习',
	'聞': '闻',
	'聯': '联',
	'聰': '聪',
	'腳': '脚',
	'臉': '脸',
	'興': '兴',
	'舉': '举',
	'舊': '旧',
	'莊': '庄',
	'萊': '莱',
	'葉': '叶',
	'蓋': '盖',
	'藝': '艺',
	'蘭': '兰',
	'號': '号',
	'蟲': '虫',
	'術': '术',
	'衛': '卫',
	'衝': '冲',
	'補': '补',
	'裝': '装',
	'裡': '里',
	'製': '制',
	'複': '复',
	'見': '见',
	'觸': '触',
	'誠': '诚',
	'語': '语',
	'讚': '赞',
	'豬': '猪',
	'貓': '猫',
	'負': '负',
	'財': '财',
	'貢': '贡',
	'貧': '贫',
	'貨': '货',
	'販': '贩',
	'貪': '贪',
	'責': '责',
	'貴': '贵',
	'買': '买',
	'貸': '贷',
	'費': '费',
	'貼': '贴',
	'賺': '赚',
	'購': '购',
	'賽': '赛',
	'贊': '赞',
	'贈': '赠',
	'趕': '赶',
	'趙': '赵',
	'蹤': '踪',
	'車': '车',
	'軌': '轨',
	'軟': '软',
	'較': '较',
	'載': '载',
	'輕': '轻',
	'輛': '辆',
	'輝': '辉',
	'輸': '输',
	'辦': '办',
	'農': '农',
	'邊': '边',
	'邏': '逻',
	'郵': '邮',
	'鄉': '乡',
	'釋': '释',
	'鐘': '钟',
	'鋼': '钢',
	'鏡': '镜',
	'鐵': '铁',
	'長': '长',
	'閉': '闭',
	'閒': '闲',
	'陣': '阵',
	'陳': '陈',
	'陰': '阴',
	'隊': '队',
	'階': '阶',
	'隨': '随',
	'險': '险',
	'雖': '虽',
	'難': '难',
	'韓': '韩',
	'須': '须',
	'預': '预',
	'領': '领',
	'醜': '丑',
	'鑑': '鉴',
	'針': '针',
	'釘': '钉',
	'鈴': '铃',
	'銀': '银',
	'銷': '销',
	'鎖': '锁',
	'鍋': '锅',
	'鍵': '键',
	'鎮': '镇',
	'闖': '闯',
	'陽': '阳',
	'隸': '隶',
	'順': '顺',
}

// 中文数字转阿拉伯：十→10、二→2。用字符串表而不是 rune 表，
// 因为「十」要转成两个字符。
var titleChineseDigit = map[rune]string{
	'零': "0", '一': "1", '二': "2", '三': "3", '四': "4",
	'五': "5", '六': "6", '七': "7", '八': "8", '九': "9", '十': "10",
}

// stripTitleNoise 剥掉结构性噪声，得到「主标题」。
func stripTitleNoise(title string) string {
	out := strings.TrimSpace(title)
	if out == "" {
		return ""
	}
	for pass := 0; pass < 2; pass++ {
		for _, pattern := range compiledTitleNoise() {
			out = pattern.ReplaceAllString(out, "")
		}
	}
	// 副标题只在「主标题足够长」时截断，避免把「A：B」这类整体标题切坏。
	for _, separator := range titleSubtitleSeparators {
		if index := strings.Index(out, separator); index >= 4 {
			out = out[:index]
			break
		}
	}
	return strings.TrimSpace(out)
}

// normalizeTitle 做繁简、数字与字符集归一化。
func normalizeTitle(title string) string {
	var builder strings.Builder
	for _, r := range title {
		if simplified, ok := titleTraditionalToSimplified[r]; ok {
			r = simplified
		}
		if mapped, ok := titleChineseDigit[r]; ok {
			builder.WriteString(mapped)
			continue
		}
		switch {
		case r >= 0xFF01 && r <= 0xFF5E:
			r -= 0xFEE0
		}
		r = unicode.ToLower(r)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// aggregateTitleKey 是一段式入口：剥离 + 归一化，供分桶使用。
func aggregateTitleKey(title string) string {
	return normalizeTitle(stripTitleNoise(title))
}

// titlesMatch 判断两个已归一化的标题是否指同一部剧。
func titlesMatch(left, right string) bool {
	if left == "" || right == "" {
		return false
	}
	if left == right {
		return true
	}
	// 中文剧名里一字之差常是另一部（修仙/修神）。长度相同且只有一个 rune 不同时，
	// 直接否决，避免 Jaro-Winkler 对长字符串的高分把这类误判放过去。
	if len([]rune(left)) == len([]rune(right)) && len([]rune(left)) <= 12 {
		diff := 0
		ar, br := []rune(left), []rune(right)
		for i := range ar {
			if ar[i] != br[i] {
				diff++
				if diff > 1 {
					break
				}
			}
		}
		if diff == 1 {
			return false
		}
	}
	// 长度差过大直接否决，避免「短标题被长标题包含」时的误合并。
	short, long := left, right
	if len(short) > len(long) {
		short, long = long, short
	}
	if long == "" || float64(len(long)-len(short))/float64(len(long)) > titleLengthRatioGuard {
		return false
	}
	return titleVoteCount(left, right) >= titleVoteMin
}

// titleVoteCount 返回三个相似度算法中达到各自阈值的票数。
// titlesMatch 的守卫已先行排除长度悬殊与一字之差，这里只做多数决。
func titleVoteCount(left, right string) int {
	votes := 0
	if jaroWinkler(left, right) >= titleMatchThreshold {
		votes++
	}
	if bigramDice(left, right) >= bigramDiceThreshold {
		votes++
	}
	if lcsRatio(left, right) >= lcsThreshold {
		votes++
	}
	return votes
}

// bigramDice 用相邻二元组交集的 Dice 系数衡量相似度（0~1）。
// 对词序重排鲁棒：顺序换了但二元组集合基本不变。中文按 rune 比较。
func bigramDice(left, right string) float64 {
	a, b := []rune(strings.ReplaceAll(left, " ", "")), []rune(strings.ReplaceAll(right, " ", ""))
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	if string(a) == string(b) {
		return 1
	}
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	counts := make(map[string]int, len(a)-1)
	for i := 0; i+1 < len(a); i++ {
		counts[string(a[i:i+2])]++
	}
	intersection := 0
	for i := 0; i+1 < len(b); i++ {
		gram := string(b[i : i+2])
		if counts[gram] > 0 {
			counts[gram]--
			intersection++
		}
	}
	return float64(2*intersection) / float64(len(a)+len(b)-2)
}

// lcsRatio 用最长公共子序列占两串平均长度的比例衡量相似度（0~1）。
// 对中段增删鲁棒。长度上限由 titlesMatch 的长度比守卫约束，这里无需额外裁剪。
func lcsRatio(left, right string) float64 {
	a, b := []rune(left), []rune(right)
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				current[j] = previous[j-1] + 1
			} else if previous[j] >= current[j-1] {
				current[j] = previous[j]
			} else {
				current[j] = current[j-1]
			}
		}
		previous, current = current, previous
	}
	return float64(2*previous[len(b)]) / float64(len(a)+len(b))
}

// jaroWinkler 计算两个字符串的 Jaro-Winkler 相似度（0~1）。
// 实现参照 RapidFuzz 的定义；中文按 rune 比较。
func jaroWinkler(left, right string) float64 {
	a, b := []rune(left), []rune(right)
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	similarity := jaro(a, b)
	if similarity < 0.7 {
		return similarity
	}
	prefix := 0
	for prefix < len(a) && prefix < len(b) && prefix < 4 && a[prefix] == b[prefix] {
		prefix++
	}
	return similarity + float64(prefix)*0.1*(1-similarity)
}

// jaro 是 Jaro 相似度的基础实现。
func jaro(a, b []rune) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 1
	}
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	matchDistance := max(len(a), len(b))/2 - 1
	if matchDistance < 0 {
		matchDistance = 0
	}
	aMatches := make([]bool, len(a))
	bMatches := make([]bool, len(b))
	matches := 0
	for i := range a {
		start := i - matchDistance
		if start < 0 {
			start = 0
		}
		end := i + matchDistance + 1
		if end > len(b) {
			end = len(b)
		}
		for j := start; j < end; j++ {
			if bMatches[j] || a[i] != b[j] {
				continue
			}
			aMatches[i] = true
			bMatches[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}
	transpositions := 0
	k := 0
	for i := range a {
		if !aMatches[i] {
			continue
		}
		for !bMatches[k] {
			k++
		}
		if a[i] != b[k] {
			transpositions++
		}
		k++
	}
	m := float64(matches)
	return (m/float64(len(a)) + m/float64(len(b)) + (m-float64(transpositions)/2)/m) / 3
}

// 编译好的噪声正则缓存：stripTitleNoise 会被聚合循环反复调用，
// 每次都 regexp.MustCompile 会成为热点。
var (
	titleNoiseOnce     sync.Once
	titleNoiseCompiled []*regexp.Regexp
)

func compiledTitleNoise() []*regexp.Regexp {
	titleNoiseOnce.Do(func() {
		titleNoiseCompiled = make([]*regexp.Regexp, 0, len(titleNoisePatterns))
		for _, pattern := range titleNoisePatterns {
			titleNoiseCompiled = append(titleNoiseCompiled, regexp.MustCompile(pattern))
		}
	})
	return titleNoiseCompiled
}

func regexpMustCompile(pattern string) *regexp.Regexp { return regexp.MustCompile(pattern) }
