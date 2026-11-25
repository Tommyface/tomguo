package core

import "strings"

// sourceSpec 是一个内置站源的静态描述。
//
// 这里是整个应用**唯一**的内置源清单：新增一个内置源必须在这张表里加一条，
// 而不是在分发函数里加一个 case。S1 之后，分发、别名解析、能力判断都查这张表。
//
// 阶段 S7 完成后，这张表会被云端清单取代（表里的每一项都变成一个 zip 模块），
// 届时本文件只剩 lookupSourceSpec 与 moduleRegistry 的联合查询。
type sourceSpec struct {
	ID     string
	Name   string
	Adults bool
	// Aliases 是除 ID 本身之外所有可被 canonicalProviderSource 识别的写法，
	// 包括域名与中文别名。匹配前统一小写。
	Aliases []string
	// Capabilities 声明该源支持的能力：catalog / detail / stream / search。
	Capabilities []string
}

// builtinSourceSpecs 是内置源清单。顺序不重要，查询走 map。
var builtinSourceSpecs = []sourceSpec{
	{ID: sourceHuangguoAI, Name: "黄果", Adults: true, Capabilities: capsAll,
		Aliases: []string{"huangguo", "huangguoai.com"}},
	{ID: sourceHuangguoVideo, Name: "黄果视频", Adults: true, Capabilities: capsAll,
		Aliases: []string{"huangguo.video"}},
	{ID: sourceHuangdou, Name: "黄豆", Adults: true, Capabilities: capsAll,
		Aliases: []string{"tideember.cc", "xqjurgek.top", "iwgsqufx.cc", "roufquunbu.cc", "ahievheved.cc", "momodrift.top", "eyeonneb.cc", "lzlukvca.cc", "glowcanvas.cc", "zuzuspot.top", "zuzucast.top"}},
	{ID: sourceHongguo, Name: "红果", Capabilities: capsAll,
		Aliases: []string{"hongguoduanju.com"}},
	{ID: sourceHuangju, Name: "剧果", Capabilities: capsAll,
		Aliases: []string{"huangju.net", "api.huangju.net"}},
	{ID: sourceYeguo, Name: "野果", Capabilities: capsAll,
		Aliases: []string{"ygdj7.com", "www.ygdj7.com", "analyze.buxefaex.cc", "delta.ygrwdsgt.cc", "yeguodj.com", "www.yeguodj.com"}},
	{ID: sourceDSD, Name: "DSD", Capabilities: capsAll,
		Aliases: []string{"dsd.com.se", "www.dsd.com.se"}},
	{ID: sourceCloudFront, Name: "CloudFront", Capabilities: capsAll},
	{ID: sourceSorani, Name: "青空次元", Capabilities: capsAll,
		Aliases: []string{"sorani.net", "www.sorani.net", "api.sorani.cc", "sorani.cc"}},
	{ID: sourceGuipian, Name: "鬼片", Capabilities: capsAll,
		Aliases: []string{"guipianwu.com", "www.guipianwu.com"}},
	{ID: sourceHanxiaoquan, Name: "韩小圈", Capabilities: capsAll,
		Aliases: []string{"jennyhow.com", "www.jennyhow.com"}},
	{ID: sourceDuanju51, Name: "51短剧", Capabilities: capsAll,
		Aliases: []string{"51短剧", "51dj", "blue.xdrgcewe.cc", "51duanju.com", "www.51duanju.com", "51duanju.app"}},
	{ID: sourceEZDMW, Name: "E站动漫", Capabilities: capsAll,
		Aliases: []string{"e站", "e站动漫", "ezdmw.org", "m.ezdmw.org", "www.ezdmw.org"}},
	{ID: sourceHema, Name: "河马剧场", Capabilities: capsAll,
		Aliases: []string{"河马", "河马短剧", "www.kuaikaw.cn", "kuaikaw.cn"}},
	{ID: sourceChengguo, Name: "橙果", Capabilities: capsAll,
		Aliases: []string{"橙果", "橙果短剧", "chengguodj.com", "www.chengguodj.com", "api2.chengguodj.com", "12.cyjoifcwr.cc"}},
	{ID: sourceHuakui, Name: "花魁", Capabilities: capsAll,
		Aliases: []string{"花魁", "花魁短剧", "huakui.net", "www.huakui.net", "huakui1.com", "huakui2.com", "huakui3.com"}},
	{ID: sourceHuangguoOfficial, Name: "黄果官网", Adults: true, Capabilities: capsAll,
		Aliases: []string{"黄果官网", "officialWebsite", "dqsibjrdsxx5r.cloudfront.net"}},
	{ID: sourceHuangguoMain, Name: "黄果主站", Adults: true, Capabilities: capsAll,
		Aliases: []string{"黄果主站", "falpius.com", "whmxun.com"}},
	{ID: sourceHuangguoSelected, Name: "黄果精选", Adults: true, Capabilities: capsAll,
		Aliases: []string{"黄果精选", "d2teten6pcf3ig.cloudfront.net"}},
	{ID: sourceHuangguoWelfare, Name: "黄果福利", Adults: true, Capabilities: capsAll,
		Aliases: []string{"黄果福利", "d24dalcptr65cw.cloudfront.net"}},
	{ID: sourceHuaguo, Name: "花果", Capabilities: capsAll,
		Aliases: []string{"花果", "花生", "www.zywest263.com", "zywest263.com"}},
	{ID: sourceWangguo, Name: "网果", Capabilities: capsAll,
		Aliases: []string{"网果", "短剧网", "www.duanju2.com", "duanju2.com"}},
	{ID: sourceFaguo, Name: "发果", Capabilities: capsAll,
		Aliases: []string{"发果", "168影院", "www.xzyx168.com", "xzyx168.com"}},
	{ID: sourceWuguo, Name: "伍果", Capabilities: capsAll,
		Aliases: []string{"伍果", "五五", "www.duanju55.com", "duanju55.com"}},
	{ID: sourceMxdm, Name: "MX动漫", Capabilities: capsAll,
		Aliases: []string{"mx", "mxdm.com", "mx动漫", "www.dcc3.com", "dcc3.com"}},
	{ID: sourceJcydm, Name: "囧次元", Capabilities: capsAll,
		Aliases: []string{"jm", "囧次元", "囧漫", "囧次元动漫", "www.jcydm1.com", "jcydm1.com"}},
}

var capsAll = []string{"catalog", "detail", "stream", "search"}

var (
	sourceSpecByID    = map[string]*sourceSpec{}
	sourceSpecByAlias = map[string]*sourceSpec{}
)

func init() {
	for index := range builtinSourceSpecs {
		spec := &builtinSourceSpecs[index]
		sourceSpecByID[spec.ID] = spec
		for _, alias := range spec.Aliases {
			key := strings.ToLower(strings.TrimSpace(alias))
			if key == "" {
				continue
			}
			sourceSpecByAlias[key] = spec
		}
	}
}

// lookupSourceSpec 按 id 或别名查内置源。传入值会被小写并去空白。
func lookupSourceSpec(source string) (*sourceSpec, bool) {
	key := strings.ToLower(strings.TrimSpace(source))
	if key == "" {
		return nil, false
	}
	if spec, ok := sourceSpecByID[key]; ok {
		return spec, true
	}
	spec, ok := sourceSpecByAlias[key]
	return spec, ok
}

// builtinSourceIDs 返回全部内置源 id，供自检与清单比对使用。
func builtinSourceIDs() []string {
	ids := make([]string, 0, len(builtinSourceSpecs))
	for _, spec := range builtinSourceSpecs {
		ids = append(ids, spec.ID)
	}
	return ids
}

// supportsCapability 判断一个内置源是否声明了某项能力。
func supportsBuiltinCapability(source, capability string) bool {
	spec, ok := lookupSourceSpec(source)
	if !ok {
		return false
	}
	for _, item := range spec.Capabilities {
		if item == capability {
			return true
		}
	}
	return false
}
