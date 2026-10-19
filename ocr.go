// OCR 文件名候选：rusto-rs（纯 Rust，PP-OCRv6 模型）静态链入主程序（仅 64 位 Windows）。
// 架子分两层：本文件是无 cgo 的编排层（后台跑、失败静默、候选进导出对话框）；
// cgo 绑定在 ocrbridge_amd64.go（真实现）与 ocrbridge_stub.go（其它构建）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// startOCRSuggest 首页扫完后自动跑 OCR 文件名候选（后台，失败静默）。
// 识别首页顶部标题性文字，给导出对话框提供文件名候选。
func (a *app) startOCRSuggest() {
	if a.ocrRunning || len(a.pages) == 0 {
		return
	}
	a.ocrRunning = true
	page := a.pages[0]
	go func() {
		names, err := ocrSuggest(page.FullPath, page.Rot)
		win.Update(func() {
			a.ocrRunning = false
			if err != nil || len(names) == 0 {
				return // 静默：候选是锦上添花，失败不打扰
			}
			a.ocrNames = names
		})
	}()
}

// ocrSuggest 从一页原图提取文件名候选：
// 解码 → 长边压到 1600（提速 + 归一）→ PNG → 识别 → 清洗/筛选/排序成候选。
//
// 整页识别而不是只裁顶部：①候选能来自页面任何位置的标题
// ②不用赌"标题在上 35% 内" ③正文段落靠 markParagraphs 成组剔掉，
// 不再依赖裁剪把正文挡在外面。代价是识别稍慢（仍远快于一次扫描）。
// 筛选/排序全在这一侧（candidateRanking），Rust 只回原始行。
func ocrSuggest(imgPath string, rot int) ([]string, error) {
	r, err := ocrSuggestRanking(imgPath, rot)
	if err != nil {
		return nil, err
	}
	return r.names(), nil
}

// ocrSuggestRanking 同 ocrSuggest，但返回完整的中间结果（调试/调参用）
func ocrSuggestRanking(imgPath string, rot int) (*ranking, error) {
	img, err := decodeImage(imgPath)
	if err != nil {
		return nil, err
	}
	if rot%360 != 0 {
		img = rotate90(img, rot)
	}
	// 长边压到 1600：提速，模型内部反正会再缩
	small := resizeToLongEdge(img, 1600)
	png, err := encodePNG(small)
	if err != nil {
		return nil, err
	}
	lines, err := ocrRecognize(png)
	if err != nil {
		return nil, err
	}
	// 几何量是相对"送进去的那张图"的像素，评分要用它的宽高做归一
	b := small.Bounds()
	return candidateRanking(lines, b.Dx(), b.Dy()), nil
}

// ── 候选生成：只看原始识别行（文字 + 置信度 + 文字框几何），全在这里定 ──
//
// 这是在"没有版面分析模型"的前提下，用几何启发式逼近"哪一行是标题"：
// 标题的特征是 **字号明显大于正文 + 位置靠上 + 横向铺得开**；
// 正文段落则相反——同字号、同左边界、上下相邻成串，据此整段剔除。
// 阈值都写成常量，想调只改这一段。调参用 --ocr-test <图片> --ocr-rank 看明细。

const (
	maxCandidates = 8  // 给用户多少条候选（对话框里 Wrap 换行，多给几条不碍事）
	maxNameRunes  = 30 // 单条候选的长度上限（再长不像标题）
)

// ocrPick 一行进入排序的中间态
type ocrPick struct {
	name   string
	score  float32
	height float32 // 文字框高度：找标题最强的信号，同高≈同一行折行
	top    float32
	width  float32
	body   bool // 属于正文段落（markParagraphs 判定）
}

// dropBody 去掉正文段落行；全被去掉时返回空（调用方据此回退）
func dropBody(picks []ocrPick) []ocrPick {
	out := make([]ocrPick, 0, len(picks))
	for _, p := range picks {
		if !p.body {
			out = append(out, p)
		}
	}
	return out
}

// ocrKept 通过硬性淘汰、等待打分的行
type ocrKept struct {
	line  ocrLine
	name  string
	body  bool // 属于正文段落（同字号同左边界成组折行）
	score float32
}

// ranking 是候选生成的中间结果，单独成形是为了能打印出来调参
type ranking struct {
	medianH  float32
	all      []ocrKept // 通过硬性淘汰的行（带 body/score）
	bodyRank []ocrKept // 被判为正文段落的行
	picks    []ocrPick // 按分数排好序的候选
	out      []string
}

func (r *ranking) names() []string { return r.out }

// candidateRanking 做完整套：中位字号 → 段落判定 → 清洗淘汰 → 打分 → 排序 → 出候选
func candidateRanking(lines []ocrLine, frameW, frameH int) *ranking {
	r := &ranking{}
	if frameW <= 0 || frameH <= 0 || len(lines) == 0 {
		return r
	}
	// 中位字号：用**全部原始行**求。字号必须相对本页比较才有意义
	// （绝对像素值随 dpi、缩放变，没法写死阈值）
	heights := make([]float32, 0, len(lines))
	for _, l := range lines {
		if l.Height > 0 {
			heights = append(heights, l.Height)
		}
	}
	if len(heights) == 0 {
		return r
	}
	sort.Slice(heights, func(i, j int) bool { return heights[i] < heights[j] })
	r.medianH = heights[len(heights)/2]
	if r.medianH <= 0 {
		return r
	}
	// 正文段落判定：必须用**全部原始行**——正文折行的某一行可能因为太长/太短
	// 而被后面的清洗淘汰，只看存活行会把一段正文看散，凑不够行数就漏判了。
	// 段落判定是纯几何问题，与"这行能不能当文件名"无关。
	all := make([]ocrKept, 0, len(lines))
	bodyAt := make([]bool, len(lines))
	for _, l := range lines {
		all = append(all, ocrKept{line: l})
	}
	markParagraphs(all, frameW, r.medianH)
	for i := range all {
		bodyAt[i] = all[i].body
	}

	// 清洗 + 硬性淘汰
	for i, l := range lines {
		if name, ok := prepareLine(l, frameW, frameH); ok {
			r.all = append(r.all, ocrKept{line: l, name: name, body: bodyAt[i]})
		}
	}
	if len(r.all) == 0 {
		return r
	}

	// 打分
	picks := make([]ocrPick, 0, len(r.all))
	for _, k := range r.all {
		if k.body {
			r.bodyRank = append(r.bodyRank, k)
		}
		if p, ok := rankLine(k, r.medianH, frameW, frameH); ok {
			picks = append(picks, p)
		}
	}
	if len(picks) == 0 {
		return r
	}
	// 判定为正文段落的行整体出局——它们已经不像标题了，留着只会占候选位。
	// 只有整页都判成正文时才回退使用（宁可给整段，也别一条都不给）。
	if nonBody := dropBody(picks); len(nonBody) > 0 {
		picks = nonBody
	}
	// 分数高的在前；同分保持识别原序（SliceStable），结果可复现
	scoreOrder := make([]int, len(picks))
	for i := range scoreOrder {
		scoreOrder[i] = i
	}
	sort.SliceStable(scoreOrder, func(a, b int) bool {
		return picks[scoreOrder[a]].score > picks[scoreOrder[b]].score
	})
	for _, i := range scoreOrder {
		r.picks = append(r.picks, picks[i])
	}

	// 折行分组：长标题折成几行时，先给"拼起来的一条"，再给各段。
	// 只认上下相邻、字号一致的（与正文段落同一套判据），
	// 免得把页面两头的孤立行硬拼成一条。
	topOrder := make([]int, len(picks))
	for i := range topOrder {
		topOrder[i] = i
	}
	sort.SliceStable(topOrder, func(a, b int) bool { return picks[topOrder[a]].top < picks[topOrder[b]].top })
	groupOf := make([]int, len(picks))
	var groups [][]int
	for _, i := range topOrder {
		if n := len(groups); n > 0 {
			g := groups[n-1]
			last := picks[g[len(g)-1]]
			if sameHeight(last.height, picks[i].height) && picks[i].top-last.top <= last.height*1.8 {
				groupOf[i] = n - 1
				groups[n-1] = append(g, i)
				continue
			}
		}
		groupOf[i] = len(groups)
		groups = append(groups, []int{i})
	}

	add := func(s string) {
		if s == "" || len(r.out) >= maxCandidates {
			return
		}
		for _, e := range r.out {
			if e == s {
				return
			}
		}
		r.out = append(r.out, s)
	}
	emitted := make([]bool, len(groups))
	for _, i := range scoreOrder { // 已按分数排序
		g := groupOf[i]
		if len(groups[g]) > 1 && !emitted[g] {
			emitted[g] = true
			parts := make([]ocrPick, 0, len(groups[g]))
			for _, j := range groups[g] {
				parts = append(parts, picks[j])
			}
			sort.SliceStable(parts, func(a, b int) bool { return parts[a].top < parts[b].top })
			add(joinTitle(parts))
		}
		add(picks[i].name)
		if len(r.out) >= maxCandidates {
			break
		}
	}
	return r
}

// markParagraphs 就地标记正文段落行。正文按行折行，特征是**上下相邻的一串**：
// 字号一致、左边界对齐、行距正常（约为字高的 1.8 倍内）。
// 三者缺一不可——只比字号和左边界的话，红头、落款、附件这些孤立的行
// 会被误并进正文。字号明显大于正文的行（标题折行）不参与。
func markParagraphs(ks []ocrKept, frameW int, medianH float32) {
	// 按从上到下取索引（不动原切片顺序）
	idx := make([]int, len(ks))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return ks[idx[a]].line.Top < ks[idx[b]].line.Top })

	tol := frameW / 50
	if tol < 2 {
		tol = 2
	}
	const minRun = 2 // 相邻两行同字号同左边界就是一整段（一页可能只识别出两行正文）

	var run []int
	flush := func() {
		if len(run) >= minRun {
			for _, i := range run {
				ks[i].body = true
			}
		}
		run = run[:0]
	}
	for _, i := range idx {
		if ks[i].line.Height > medianH*1.4 {
			flush() // 大字号：标题，不参与成段
			continue
		}
		if len(run) == 0 {
			run = append(run, i)
			continue
		}
		prev := ks[run[len(run)-1]].line
		cur := ks[i].line
		if sameHeight(prev.Height, cur.Height) &&
			intAbs(int(prev.Left)-int(cur.Left)) <= tol &&
			cur.Top-prev.Top <= prev.Height*1.8 {
			run = append(run, i)
		} else {
			flush()
			run = append(run, i)
		}
	}
	flush()
}

func sameHeight(a, b float32) bool {
	if a <= 0 || b <= 0 {
		return false
	}
	d, m := a-b, a
	if d < 0 {
		d = -d
	}
	if b > m {
		m = b
	}
	return d/m <= 0.12
}

func intAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// joinTitle 把折行的标题段按顺序拼成一条。中英文之间不留多余空格，
// 超出长度上限就停在能放下的那一段。
func joinTitle(parts []ocrPick) string {
	var b strings.Builder
	prevTop, prevH := float32(-1), float32(0)
	for _, p := range parts {
		// 不是紧挨着的下一行（中间隔了正文）就断开，别把不相干的两行粘一起
		if prevTop >= 0 && p.top > prevTop+prevH*2.2 {
			break
		}
		if utf8.RuneCountInString(b.String())+utf8.RuneCountInString(p.name) > maxNameRunes {
			break
		}
		if b.Len() > 0 && needsSpaceBetween(b.String(), p.name) {
			b.WriteRune(' ')
		}
		b.WriteString(p.name)
		prevTop, prevH = p.top, p.height
	}
	return b.String()
}

// needsSpaceBetween 拼接处是否需要空格：两边都是汉字就不要
func needsSpaceBetween(a, b string) bool {
	ar, _ := utf8.DecodeLastRuneInString(a)
	br, _ := utf8.DecodeRuneInString(b)
	return !(isCJK(ar) && isCJK(br))
}

// prepareLine 清洗 + 硬性淘汰。返回清洗后的候选名，ok=false 表示这行直接不要。
// 淘汰的都是"无论版面如何都不像标题"的：太短/太长、纯数字、符号噪声、页脚页码。
func prepareLine(l ocrLine, frameW, frameH int) (string, bool) {
	name := normalizeOCRText(l.Text)
	if name == "" || l.Height <= 0 || frameW <= 0 || frameH <= 0 {
		return "", false
	}
	// 字段行（"甲方：某某公司"）取冒号后的值当候选——标签本身对文件名没意义，
	// 而值（公司名、项目名）往往正好是用户想用的名字。标签过长说明是标题里的
	// 冒号而不是字段前缀，不动。是否字段行要在剥离前判断。
	if idx := labelColonIndex(name); idx >= 0 && idx <= 6 {
		if v := strings.TrimSpace(string([]rune(name)[idx+1:])); v != "" {
			name = v
		}
	}
	runes := utf8.RuneCountInString(name)
	if runes < 2 || runes > maxNameRunes {
		return "", false
	}
	var cjk, letter, digit int
	for _, r := range name {
		switch {
		case isCJK(r):
			cjk++
		case unicode.IsDigit(r):
			digit++
		case unicode.IsLetter(r):
			letter++
		}
	}
	if cjk+letter == 0 {
		return "", false // 纯数字/符号：页码、分隔线
	}
	if float32(digit)/float32(runes) > 0.5 {
		return "", false // 数字占一半以上：日期、流水号
	}
	if float32(cjk+letter)/float32(runes) < 0.7 {
		return "", false // 符号噪声太多：识别质量差的行
	}
	return name, true
}

// rankLine 给清洗后的行打分。medianH 是本页中位字号（相对比较的基准）。
// 各分项归一到 0..1，权重写在表达式的系数里，改口味就改这一段。
func rankLine(k ocrKept, medianH float32, frameW, frameH int) (ocrPick, bool) {
	l, name := k.line, k.name
	field := false
	if idx := labelColonIndex(name); idx >= 0 && idx <= 6 {
		field = true
	}
	runes := utf8.RuneCountInString(name)

	// 字号：标题会比正文大一截——这是最强的信号，用"相对本页中位字号"度量，
	// 免得写死的像素阈值随 dpi/缩放失效。1.0 倍=0 分，1.8 倍及以上=满分。
	hScore := clamp01((l.Height/medianH - 1.0) / (1.8 - 1.0))
	// 位置：越靠上越像标题（权重给低——红头/页眉也在最上面，靠字号压下去）
	pScore := clamp01(1 - (l.Top/float32(frameH)-0.02)/(0.45-0.02))
	// 横向铺开程度：标题一般占版心相当宽
	wScore := clamp01(l.Width / float32(frameW) / 0.55)
	// 置信度：识别得越准越可信
	cScore := clamp01((l.Score - 0.60) / (0.95 - 0.60))

	score := 4.0*hScore + 1.5*pScore + 1.0*wScore + 1.0*cScore
	// 太短（"附件"）不像标题；过长（整段正文）也压一点——但公文标题本就常见
	// 20-30 字，所以上限放宽到 28，免得把正经标题砍掉
	switch {
	case runes < 4:
		score *= 0.4
	case runes > 28:
		score *= 0.5
	}
	// 字段行：值可以当候选，但名次要排在标题后面
	if field {
		score *= 0.35
	}
	// 正文段落行：成组折行的整段文字，不是标题
	if k.body {
		score *= 0.25
	}
	// 公文的固定抬头/落款词：不硬禁（万一整页只有它），只重罚
	if hasBoilerplate(name) {
		score *= 0.3
	}
	return ocrPick{name: name, score: score, height: l.Height, top: l.Top, width: l.Width, body: k.body}, true
}

// labelColonIndex 返回第一个中/英文冒号的字符位置，没有返回 -1
func labelColonIndex(name string) int {
	for i, r := range []rune(name) {
		if r == '：' || r == ':' {
			return i
		}
	}
	return -1
}

// boilerplate 公文里反复出现、但几乎不可能是标题的词（前缀匹配）
var boilerplate = []string{
	"附件", "抄送", "抄报", "签发", "主题词", "密级", "编号", "页码", "合同编号",
	"甲方", "乙方", "丙方", "日期", "名称", "单位", "地址", "电话", "传真",
	"邮编", "网址", "邮箱", "第", "共",
}

// boilerplateSuffix 同样反复出现、但只出现在结尾的词。带长度限制：
// "…文件"是红头（机关名+文件），但长标题里出现"文件"是正常的
var boilerplateSuffix = []string{"文件", "目录", "附表"}

func hasBoilerplate(name string) bool {
	for _, w := range boilerplate {
		if strings.HasPrefix(name, w) {
			return true
		}
	}
	for _, w := range boilerplateSuffix {
		if strings.HasSuffix(name, w) && utf8.RuneCountInString(name) <= 14 {
			return true
		}
	}
	return false
}

// normalizeOCRText 把识别文本清成能当文件名的样子：
// 去非法字符与控制符（`\/:*?"<>|`）、压空白、
// 去掉汉字之间 OCR 硬塞进来的空格、削掉句末标点。
func normalizeOCRText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < 0x20:
			// 控制符丢弃
		case r == '　': // 全角空格
			b.WriteRune(' ')
		case r == ' ': // 不换行空格
			b.WriteRune(' ')
		case strings.ContainsRune(`\/:*?"<>|`, r), unicode.IsSpace(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	src := []rune(b.String())
	out := make([]rune, 0, len(src))
	for i, r := range src {
		if r == ' ' {
			if len(out) == 0 || out[len(out)-1] == ' ' {
				continue // 行首空格 / 连续空格
			}
			var next rune
			if i+1 < len(src) {
				next = src[i+1]
			}
			if isCJK(out[len(out)-1]) && isCJK(next) {
				continue // "关 于" → "关于"
			}
		}
		out = append(out, r)
	}
	// 削首尾的空白与句末标点（OCR 很爱在标题末尾加个句号）
	for len(out) > 0 && (out[len(out)-1] == ' ' || strings.ContainsRune("。．，、；：,.;:!?！？", out[len(out)-1])) {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && out[0] == ' ' {
		out = out[1:]
	}
	return string(out)
}

// isCJK 汉字/假名/谚文（判定"字与字之间不用空格"用）
func isCJK(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF, // 中日韩统一表意
		r >= 0x3400 && r <= 0x4DBF, // 扩展 A
		r >= 0xF900 && r <= 0xFAFF, // 兼容表意
		r >= 0x3040 && r <= 0x30FF, // 平假名 / 片假名
		r >= 0xAC00 && r <= 0xD7AF: // 谚文
		return true
	}
	return false
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// applyOCRName 应用 OCR 候选到文件名输入框：第一次替换，后续追加。
func (a *app) applyOCRName(name string) {
	if a.ocrPicked && a.exportFilename != "" {
		a.exportFilename += name
	} else {
		a.exportFilename = name
	}
	a.ocrPicked = true
}

// modelDir OCR 模型三件套所在目录：从内嵌资产释放到临时目录（单文件分发）。
// 释放失败时退回 exe 同目录的 models/（兼容老式并排摆放）。
func modelDir() string {
	if err := ensureAssets(); err == nil {
		return filepath.Join(tempRoot, "models")
	}
	exe, err := exePath()
	if err != nil {
		return "models"
	}
	return filepath.Join(filepath.Dir(exe), "models")
}

// runOcrTest --ocr-test 的实现：对一张图片走完整候选管线并打印（验收用）。
// --ocr-raw   打印全图原始识别行（不裁顶部、不过滤），看识别质量
// --ocr-rank  打印候选打分明细（中位字号、每行的分数与惩罚项），调参用
func runOcrTest(imgPath string) {
	mode := ""
	if len(os.Args) > 3 {
		mode = os.Args[3]
	}
	t0 := time.Now()
	switch mode {
	case "--ocr-raw":
		// 原始行：不裁顶部、不过滤，全图识别
		img, err := decodeImage(imgPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		small := resizeToLongEdge(img, 1600)
		png, err := encodePNG(small)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		lines, err := ocrRecognize(png)
		if err != nil {
			fmt.Fprintln(os.Stderr, "识别失败:", err)
			os.Exit(1)
		}
		b := small.Bounds()
		fmt.Printf("全图识别 %d 行（%dx%d，耗时 %v）：\n", len(lines), b.Dx(), b.Dy(), time.Since(t0))
		for _, l := range lines {
			fmt.Printf("  [top=%4.0f left=%4.0f w=%4.0f h=%4.0f score=%.2f] %s\n",
				l.Top, l.Left, l.Width, l.Height, l.Score, l.Text)
		}
	case "--ocr-rank":
		r, err := ocrSuggestRanking(imgPath, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		fmt.Printf("耗时 %v，中位字号 %.0f\n", time.Since(t0), r.medianH)
		fmt.Printf("通过淘汰 %d 行，其中正文段落 %d 行：\n", len(r.all), len(r.bodyRank))
		for _, k := range r.all {
			mark := "    "
			if k.body {
				mark = "正文"
			}
			fmt.Printf("  %s [h=%4.0f top=%4.0f w=%4.0f s=%.2f] %s\n",
				mark, k.line.Height, k.line.Top, k.line.Width, k.line.Score, k.name)
		}
		fmt.Printf("排序（前 12）：\n")
		for i, p := range r.picks {
			if i >= 12 {
				break
			}
			fmt.Printf("  %5.2f  h=%4.0f  %s\n", p.score, p.height, p.name)
		}
		fmt.Printf("最终候选 %d 条:\n", len(r.out))
		for i, n := range r.out {
			fmt.Printf("  %d. %s\n", i+1, n)
		}
	default:
		names, err := ocrSuggest(imgPath, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, "失败:", err)
			os.Exit(1)
		}
		fmt.Printf("耗时 %v\n候选 %d 条:\n", time.Since(t0), len(names))
		for i, n := range names {
			fmt.Printf("  %d. %s\n", i+1, n)
		}
	}
}
