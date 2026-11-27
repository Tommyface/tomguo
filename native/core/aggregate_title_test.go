package core

import "testing"

func TestAggregateTitleNormalization(t *testing.T) {
	cases := []struct{ input, want string }{
		{"总裁夫人第二季 全集高清", "总裁夫人"},
		{"總裁夫人第2季", "总裁夫人"},
		{"ＡＢＣ　(def)", "abcdef"},
		{"  重生之都市修仙  （2026）  ", "重生之都市修仙"},
	}
	for _, tc := range cases {
		if got := aggregateTitleKey(tc.input); got != tc.want {
			t.Errorf("aggregateTitleKey(%q) = %q，期望 %q", tc.input, got, tc.want)
		}
	}
}

func TestTitlesMatchPositive(t *testing.T) {
	pairs := [][2]string{
		{"总裁夫人第二季", "总裁夫人第2季"},
		{"总裁夫人", "總裁夫人"},
		{"闪婚总裁大人全集高清", "闪婚总裁大人"},
		{"我的美女老板（2026）", "我的美女老板"},
	}
	for _, pair := range pairs {
		left, right := aggregateTitleKey(pair[0]), aggregateTitleKey(pair[1])
		if !titlesMatch(left, right) {
			t.Errorf("%q 与 %q 应判为同一部（%q vs %q）", pair[0], pair[1], left, right)
		}
	}
}

func TestTitlesMatchNegative(t *testing.T) {
	pairs := [][2]string{
		{"重生之都市修仙", "重生之都市修神"},
		{"总裁夫人", "总裁的女儿"},
		{"第一集", "第二集"},
	}
	for _, pair := range pairs {
		left, right := aggregateTitleKey(pair[0]), aggregateTitleKey(pair[1])
		if titlesMatch(left, right) {
			t.Errorf("%q 与 %q 不应合并（%q vs %q）", pair[0], pair[1], left, right)
		}
	}
}

func TestJaroWinklerKnownValues(t *testing.T) {
	if got := jaroWinkler("MARTHA", "MARHTA"); got < 0.96 || got > 0.97 {
		t.Fatalf("MARTHA/MARHTA = %f，期望约 0.961", got)
	}
	if got := jaroWinkler("abc", "xyz"); got != 0 {
		t.Fatalf("abc/xyz = %f，期望 0", got)
	}
	if got := jaroWinkler("", ""); got != 1 {
		t.Fatalf("空串 = %f，期望 1", got)
	}
}

func TestAggregateTitleCorpusAccuracy(t *testing.T) {
	// 200 组：100 组应合并、100 组不应合并。
	// 正例覆盖副标题/画质词/繁简/季数写法；负例覆盖近似但不同的剧名。
	suffixes := []string{"", " 全集高清", "（2026）", " 国语", " HD", " 抢先", " 蓝光"}
	seasons := []string{"", "第二季", "第2季", " 第2部"}
	positives := [][2]string{}
	negatives := [][2]string{}
	positiveNames := []string{
		"隐婚老公太宠我",
		"总裁夫人", "闪婚总裁大人", "我的美女老板", "重生之都市修仙", "离婚后总裁追妻",
		"替身甜妻", "医妃权倾天下", "穿越古代种田", "豪门千金归来", "隐婚老公太宠我",
	}
	negativeNames := [][2]string{
		{"总裁夫人", "总裁的女儿"}, {"重生之都市修仙", "重生之都市修神"},
		{"闪婚总裁大人", "闪婚总裁少爷"}, {"我的美女老板", "我的帅气老板"},
		{"离婚后总裁追妻", "离婚后总裁娶妻"}, {"替身甜妻", "替身娇妻"},
		{"医妃权倾天下", "医妃权倾后宫"}, {"穿越古代种田", "穿越现代种田"},
		{"豪门千金归来", "豪门千金回乡"}, {"隐婚老公太宠我", "隐婚老公太宠她"},
		{"总裁夫人", "总裁夫人的秘密"}, {"重生之都市修仙", "重生之都市修真"},
		{"闪婚总裁大人", "闪婚总裁千金"}, {"我的美女老板", "我的美女上司"},
		{"替身甜妻", "替身甜女"}, {"医妃权倾天下", "医妃权倾朝野"},
	}
	for _, name := range positiveNames {
		for _, variant := range suffixes {
			positives = append(positives, [2]string{name, name + variant})
		}
	}
	for _, name := range positiveNames {
		for _, season := range seasons {
			positives = append(positives, [2]string{name + season, name})
		}
	}
	for _, pair := range negativeNames {
		for _, variant := range suffixes {
			negatives = append(negatives, [2]string{pair[0], pair[1] + variant})
		}
	}
	if len(positives) < 100 || len(negatives) < 100 {
		t.Fatalf("用例不足：正例 %d，负例 %d", len(positives), len(negatives))
	}
	positives, negatives = positives[:100], negatives[:100]

	ok, wrong := 0, 0
	for _, pair := range positives {
		if titlesMatch(aggregateTitleKey(pair[0]), aggregateTitleKey(pair[1])) {
			ok++
		} else {
			wrong++
			t.Logf("漏合并：%q / %q", pair[0], pair[1])
		}
	}
	for _, pair := range negatives {
		if !titlesMatch(aggregateTitleKey(pair[0]), aggregateTitleKey(pair[1])) {
			ok++
		} else {
			wrong++
			t.Logf("误合并：%q / %q", pair[0], pair[1])
		}
	}
	total := len(positives) + len(negatives)
	accuracy := float64(ok) / float64(total)
	t.Logf("合并用例准确率 %.1f%%（%d/%d）", accuracy*100, ok, total)
	if accuracy < 0.95 || wrong > 2 {
		t.Fatalf("准确率 %.1f%%、误合并 %d 组，未达 95%%/≤2 验收线", accuracy*100, wrong)
	}
}

func TestTitleVoteAlgorithms(t *testing.T) {
	if got := bigramDice("总裁的替身新娘", "替身新娘总裁"); got < 0.6 {
		t.Errorf("词序重排 bigramDice=%f 应较高", got)
	}
	if got := lcsRatio("总裁夫人", "总裁夫人的秘密"); got <= 0 || got >= 1 {
		t.Errorf("lcsRatio=%f 应在 0~1 之间", got)
	}
	if bigramDice("", "总裁") != 0 || lcsRatio("总裁", "") != 0 {
		t.Error("空串相似度应为 0")
	}
}

func TestTitlesMatchReordered(t *testing.T) {
	// 单 Jaro-Winkler 对词序重排偏低（< 0.92 会漏判），三算法投票应补上这类盲区。
	pairs := [][2]string{
		{"我的总裁老公大人", "总裁老公大人我的"},
	}
	for _, pair := range pairs {
		left, right := aggregateTitleKey(pair[0]), aggregateTitleKey(pair[1])
		if jaroWinkler(left, right) >= titleMatchThreshold {
			t.Logf("%q / %q 单 Jaro-Winkler 已足够，该用例不体现投票价值", pair[0], pair[1])
			continue
		}
		if !titlesMatch(left, right) {
			t.Errorf("%q 与 %q 词序重排应判为同一部（votes=%d dice=%.3f lcs=%.3f jw=%.3f）",
				pair[0], pair[1], titleVoteCount(left, right),
				bigramDice(left, right), lcsRatio(left, right), jaroWinkler(left, right))
		}
	}
}
