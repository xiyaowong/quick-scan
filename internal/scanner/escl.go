// eSCL 客户端。只用 IPv4；端口与路径来自 mDNS TXT，缺省 80 / "eSCL"。
package scanner

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultESCLPath = "eSCL"
	probeTimeout    = 6 * time.Second   // 读能力：设备在线就该秒回
	jobTimeout      = 15 * time.Second  // 建任务
	pageTimeout     = 120 * time.Second // 取一张：600dpi 冷启动也要等
	defaultVersion  = "2.63"
)

// scannerHTTP 直连局域网设备并禁用代理；否则系统代理会把 192.168.x.x 的请求也劫走。
var scannerHTTP = &http.Client{
	Transport: &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	},
}

type scannerCapabilities struct {
	Version      string
	MakeAndModel string
	Platen       *inputCaps // 玻璃板
	ADF          *inputCaps // 进纸口（nil = 没有）
	Formats      []string   // 两种输入源的格式并集
}

// 尺寸单位 1/300 英寸。
type inputCaps struct {
	MinWidth, MaxWidth   int
	MinHeight, MaxHeight int
	Colors               []string // "color"|"gray"|"bw"（界面用的键）
	RawModes             []string // 设备原样上报的 ColorMode（下发任务时用）
	Resolutions          []float32
	Formats              []string
	Intents              []string // eSCL Intent 原样值（Document / TextAndGraphic / Photo …）
}

type capsXML struct {
	XMLName      xml.Name   `xml:"ScannerCapabilities"`
	Version      string     `xml:"Version"`
	MakeAndModel string     `xml:"MakeAndModel"`
	Platen       *platenXML `xml:"Platen"`
	ADF          *adfXML    `xml:"ADF"`
}

type platenXML struct {
	Caps *inputCapsXML `xml:"PlatenInputCaps"`
}

type adfXML struct {
	Simplex *inputCapsXML `xml:"AdfSimplexInputCaps"`
	Duplex  *inputCapsXML `xml:"AdfDuplexInputCaps"`
}

type inputCapsXML struct {
	MinWidth  int `xml:"MinWidth"`
	MaxWidth  int `xml:"MaxWidth"`
	MinHeight int `xml:"MinHeight"`
	MaxHeight int `xml:"MaxHeight"`

	// ColorModes/DocumentFormats 可能直接挂在 InputCaps 下，也可能在 SettingProfiles 里，两处都收。
	ColorModes      []string      `xml:"ColorModes>ColorMode"`
	DocumentFormats []string      `xml:"DocumentFormats>DocumentFormat"`
	Intents         []string      `xml:"SupportedIntents>Intent"`
	Profiles        []profileXML  `xml:"SettingProfiles>SettingProfile"`
	Discrete        []discreteXML `xml:"SupportedResolutions>DiscreteResolutions>DiscreteResolution"`
	Range           *resRangeXML  `xml:"SupportedResolutions>ResolutionRange"`
}

type profileXML struct {
	ColorModes      []string      `xml:"ColorModes>ColorMode"`
	DocumentFormats []string      `xml:"DocumentFormats>DocumentFormat"`
	Discrete        []discreteXML `xml:"SupportedResolutions>DiscreteResolutions>DiscreteResolution"`
	Range           *resRangeXML  `xml:"SupportedResolutions>ResolutionRange"`
}

type discreteXML struct {
	X int `xml:"XResolution"`
	Y int `xml:"YResolution"`
}

type resRangeXML struct {
	X *axisRangeXML `xml:"XResolutionRange"`
	Y *axisRangeXML `xml:"YResolutionRange"`
}

type axisRangeXML struct {
	Min  int `xml:"Min"`
	Max  int `xml:"Max"`
	Step int `xml:"Step"`
}

func parseCapabilities(data []byte) (*scannerCapabilities, error) {
	var raw capsXML
	if err := xml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	c := &scannerCapabilities{
		Version:      sanitizeVersion(raw.Version),
		MakeAndModel: strings.TrimSpace(raw.MakeAndModel),
	}
	if raw.Platen != nil && raw.Platen.Caps != nil {
		c.Platen = buildInputCaps(raw.Platen.Caps)
	}
	if raw.ADF != nil {
		c.ADF = buildInputCaps(raw.ADF.Simplex)
		if c.ADF == nil {
			c.ADF = buildInputCaps(raw.ADF.Duplex)
		}
		// ADF 节点存在但没能力细节：也说明有进纸口
		if c.ADF == nil {
			c.ADF = &inputCaps{}
		}
	}
	for _, in := range []*inputCaps{c.Platen, c.ADF} {
		if in != nil {
			c.Formats = appendUnique(c.Formats, in.Formats...)
		}
	}
	return c, nil
}

// 版本会原样进 XML 请求体，别让设备上的怪字符带进去。
func sanitizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return defaultVersion
	}
	for _, r := range v {
		if (r < '0' || r > '9') && r != '.' {
			return defaultVersion
		}
	}
	return v
}

func buildInputCaps(raw *inputCapsXML) *inputCaps {
	if raw == nil {
		return nil
	}
	in := &inputCaps{
		MinWidth:  raw.MinWidth,
		MaxWidth:  raw.MaxWidth,
		MinHeight: raw.MinHeight,
		MaxHeight: raw.MaxHeight,
	}
	modes := append([]string{}, raw.ColorModes...)
	formats := append([]string{}, raw.DocumentFormats...)
	for _, p := range raw.Profiles {
		modes = append(modes, p.ColorModes...)
		formats = append(formats, p.DocumentFormats...)
	}
	for _, m := range modes {
		if key := colorKey(m); key != "" {
			in.Colors = appendUnique(in.Colors, key)
			in.RawModes = appendUnique(in.RawModes, m)
		}
	}
	// 固定顺序：彩色 → 灰度 → 黑白（界面按这个顺序显示）
	in.Colors = orderColors(in.Colors)
	in.Formats = appendUnique(nil, formats...)
	in.Intents = appendUnique(nil, raw.Intents...)

	// 分辨率也可能挂在 SettingProfile 里（HP 116w 就是这种写法），和上面一样两处都收。
	discrete := append([]discreteXML{}, raw.Discrete...)
	ranges := []*resRangeXML{raw.Range}
	for _, p := range raw.Profiles {
		discrete = append(discrete, p.Discrete...)
		ranges = append(ranges, p.Range)
	}
	for _, d := range discrete {
		if d.X > 0 {
			in.Resolutions = appendUniqueFloat(in.Resolutions, float32(d.X))
		}
	}
	if len(in.Resolutions) == 0 {
		for _, r := range ranges {
			if r != nil && r.X != nil {
				in.Resolutions = appendRangeResolutions(in.Resolutions, r.X)
				break
			}
		}
	}
	slices.Sort(in.Resolutions)
	return in
}

func appendRangeResolutions(dst []float32, rg *axisRangeXML) []float32 {
	for _, dpi := range []int{300, 600} {
		if dpi >= rg.Min && dpi <= rg.Max && (rg.Step <= 0 || (dpi-rg.Min)%rg.Step == 0) {
			dst = append(dst, float32(dpi))
		}
	}
	return dst
}

func colorKey(mode string) string {
	switch {
	case strings.HasPrefix(mode, "BlackAndWhite"):
		return "bw"
	case strings.HasPrefix(mode, "Grayscale"):
		return "gray"
	case strings.HasPrefix(mode, "RGB"):
		return "color"
	}
	return ""
}

func orderColors(keys []string) []string {
	var out []string
	for _, k := range []string{"color", "gray", "bw"} {
		if slices.Contains(keys, k) {
			out = append(out, k)
		}
	}
	return out
}

func appendUnique(dst []string, vs ...string) []string {
	for _, v := range vs {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(dst, v) {
			dst = append(dst, v)
		}
	}
	return dst
}

func appendUniqueFloat(dst []float32, v float32) []float32 {
	if slices.Contains(dst, v) {
		return dst
	}
	return append(dst, v)
}

// 界面向 eSCL 传的语义键。具体协议的 ColorMode 名由设备原样上报值决定。
const (
	ColorModeColor = "color"
	ColorModeGray  = "gray"
	ColorModeBW    = "bw"

	PaperA4     = "a4"
	PaperA5     = "a5"
	PaperLetter = "letter"

	// IntentDocument 是 eSCL 规范里的默认用途；设备另报别的时以设备为主。
	IntentDocument = "Document"
)

var paperUnits = map[string][2]int{
	PaperA4:     {2480, 3508},
	PaperA5:     {1748, 2480},
	PaperLetter: {2550, 3300},
}

// 未知尺寸按 A4 算：键来自界面，万一传岔了也不能下发 0×0 的扫描区域。
func paperUnit(size string) [2]int {
	if u, ok := paperUnits[size]; ok {
		return u
	}
	return paperUnits[PaperA4]
}

// 允许横竖两个方向；Max 缺失 = 不限制。
func (in *inputCaps) supportsSize(s string) bool {
	if in == nil || in.MaxWidth <= 0 || in.MaxHeight <= 0 {
		return true
	}
	u := paperUnit(s)
	w, h := u[0], u[1]
	const slack = 8 // 各厂商对 A4 的换算差几个单位
	fit := func(a, b int) bool { return a <= in.MaxWidth+slack && b <= in.MaxHeight+slack }
	return fit(w, h) || fit(h, w)
}

func (in *inputCaps) supportsColor(key string) bool {
	if in == nil || len(in.Colors) == 0 {
		return true
	}
	return slices.Contains(in.Colors, key)
}

func (c *scannerCapabilities) supportsColor(key string) bool {
	if c.Platen == nil && c.ADF == nil {
		return true // 能力没上报：不限制
	}
	return (c.Platen != nil && c.Platen.supportsColor(key)) ||
		(c.ADF != nil && c.ADF.supportsColor(key))
}

func (c *scannerCapabilities) supportsSize(s string) bool {
	if c.Platen == nil && c.ADF == nil {
		return true // 能力没上报：不限制
	}
	return (c.Platen != nil && c.Platen.supportsSize(s)) ||
		(c.ADF != nil && c.ADF.supportsSize(s))
}

func (c *scannerCapabilities) resolutions() []float32 {
	var out []float32
	for _, in := range []*inputCaps{c.Platen, c.ADF} {
		if in == nil {
			continue
		}
		for _, r := range in.Resolutions {
			out = appendUniqueFloat(out, r)
		}
	}
	slices.Sort(out)
	return out
}

func (c *scannerCapabilities) intents() []string {
	var out []string
	for _, in := range []*inputCaps{c.Platen, c.ADF} {
		if in != nil {
			out = appendUnique(out, in.Intents...)
		}
	}
	return out
}

// JPEG > PNG；只报 PDF 时返回空。
func pickFormat(formats []string) string {
	for _, want := range []string{"image/jpeg", "image/jpg", "image/png"} {
		for _, f := range formats {
			if strings.EqualFold(strings.TrimSpace(f), want) {
				return want
			}
		}
	}
	for _, f := range formats {
		lf := strings.ToLower(f)
		if strings.HasPrefix(lf, "image/jpeg") || strings.HasPrefix(lf, "image/jpg") {
			return "image/jpeg"
		}
		if strings.HasPrefix(lf, "image/png") {
			return "image/png"
		}
	}
	return ""
}

func Probe(ctx context.Context, d *DeviceInfo) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.capabilitiesURL(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/xml, text/xml")
	resp, err := scannerHTTP.Do(req)
	if err != nil {
		return fmt.Errorf("连不上设备：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("设备返回 HTTP %d", resp.StatusCode)
	}

	caps, perr := parseCapabilities(body)
	if perr != nil {
		// 解析失败不算离线：选项全开也能扫，只是没法按设备能力收窄。
		caps = &scannerCapabilities{Version: defaultVersion}
	}
	applyCaps(d, caps)
	return nil
}

// 界面只读 d 上的字段，不碰 caps。
func applyCaps(d *DeviceInfo, caps *scannerCapabilities) {
	d.caps = caps
	d.Online = true
	if d.Name == "" && caps.MakeAndModel != "" {
		d.Name = caps.MakeAndModel
	}
	d.Colors = supportedColors(caps)
	d.Resolutions = caps.resolutions()
	d.Intents = caps.intents()
	d.Sizes = supportedSizes(caps)
	d.Feeder = caps.ADF != nil
}

// 返回空切片而不是 nil：这些值会原样进缓存 JSON，nil 会写成 null。
func supportedColors(caps *scannerCapabilities) []string {
	out := []string{}
	for _, c := range []string{ColorModeColor, ColorModeGray, ColorModeBW} {
		if caps.supportsColor(c) {
			out = append(out, c)
		}
	}
	return out
}

func supportedSizes(caps *scannerCapabilities) []string {
	out := []string{}
	for _, s := range []string{PaperA4, PaperA5, PaperLetter} {
		if caps.supportsSize(s) {
			out = append(out, s)
		}
	}
	return out
}

type Options struct {
	Color  string // color / gray / bw
	DPI    int
	Size   string // a4 / a5 / letter
	Feeder bool
	Intent string // eSCL Intent；空 = Document
}

type ScanSink struct {
	Dir string
	// 0..1 的估算进度。
	Progress func(float64)
	// OnPage 在扫描 goroutine 里每落盘一页调一次：扫描中途崩溃也不会丢掉已经下完的页。
	OnPage func(path string)
}

func Scan(ctx context.Context, d *DeviceInfo, opts Options, sink ScanSink) (_ []string, err error) {
	report := func(f float64) {
		if sink.Progress != nil {
			sink.Progress(f)
		}
	}
	if d.caps == nil {
		if err := Probe(ctx, d); err != nil {
			return nil, err
		}
	}
	caps := d.caps
	in := caps.Platen
	source := "Platen"
	if opts.Feeder {
		in = caps.ADF
		source = "ADF"
	}
	if in == nil {
		return nil, errors.New("这台设备没有进纸口")
	}

	set := scanSettings{
		Version:   caps.Version,
		Format:    pickFormat(append(append([]string{}, in.Formats...), caps.Formats...)),
		ColorMode: pickColorMode(in, opts.Color),
		Source:    source,
		Opts:      opts,
		Caps:      in,
	}
	if set.Format == "" {
		return nil, errors.New("设备不支持扫描成图片（只报了 PDF 等格式）")
	}
	set.Opts.Intent = pickIntent(in, opts.Intent)
	if set.Version == "" {
		set.Version = defaultVersion
	}

	jobURL, err := createScanJob(ctx, d, buildScanSettings(set))
	if err != nil {
		return nil, err
	}
	// 半路退出（失败或用户停止）都删任务。正常扫完由设备自己收尾，
	// 删了反而被记成「用户取消」。
	defer func() {
		if err != nil || ctx.Err() != nil {
			cancelScanJob(jobURL)
		}
	}()
	report(0.02)

	expect := pageEstimate(opts)
	// 进纸口不知道还剩几张：每页只推进剩余量的一段，扫完再补到 100%。
	span, base := 1.0, 0.0
	if opts.Feeder {
		span = 0.9
	}

	ext := pageExt(set.Format)
	var files []string
	for i := 1; ; i++ {
		data, ct, err := nextDocument(ctx, jobURL, expect, func(f float64) {
			report(base + (1-base)*span*f)
		})
		if err != nil {
			if ctx.Err() != nil {
				// 用户停止：已落盘的页照常交付，任务交给 defer 里的 DELETE 收尾。
				return files, nil
			}
			return nil, err
		}
		if data == nil {
			break // 404：没有下一张了
		}
		if strings.Contains(ct, "pdf") {
			return nil, errors.New("设备返回了 PDF 格式，无法在界面里显示")
		}
		path := filepath.Join(sink.Dir, fmt.Sprintf("page_%d_%d%s", time.Now().UnixMilli(), i, ext))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, fmt.Errorf("保存扫描页失败：%w", err)
		}
		files = append(files, path)
		if sink.OnPage != nil {
			sink.OnPage(path)
		}
		if !opts.Feeder {
			break // 玻璃板一次一张
		}
		base += (1 - base) * span
		report(base)
	}
	if len(files) == 0 && ctx.Err() == nil {
		return nil, errors.New("扫描仪没有返回任何页面（可能缺纸）")
	}
	report(1)
	return files, nil
}

func pageExt(format string) string {
	if format == "image/png" {
		return ".png"
	}
	return ".jpg"
}

// 像素数 ÷ 经验吞吐，彩色最慢。
func pageEstimate(opts Options) time.Duration {
	u := paperUnit(opts.Size)
	scale := float64(opts.DPI) / 300
	px := float64(u[0]*u[1]) * scale * scale
	rate := 1.6e6 // 像素/秒
	switch opts.Color {
	case ColorModeGray:
		rate = 3.2e6
	case ColorModeBW:
		rate = 8e6
	}
	// 再快也得留一秒，否则进度条一眨眼就到头，看不出在扫。
	return max(time.Second, time.Duration(px/rate*float64(time.Second)))
}

// 前段爬得快，越往后越慢，留 10% 给下载。
func scanEstimate(elapsed, expect time.Duration) float64 {
	if expect <= 0 {
		expect = 6 * time.Second
	}
	return 0.9 * (1 - math.Exp(-1.6*elapsed.Seconds()/expect.Seconds()))
}

func (d *DeviceInfo) capabilitiesURL() string {
	return d.resourceURL("ScannerCapabilities")
}

func (d *DeviceInfo) scanJobsURL() string {
	return d.resourceURL("ScanJobs")
}

func (d *DeviceInfo) resourceURL(name string) string {
	port := d.Port
	if port == 0 {
		port = 80
	}
	path := strings.Trim(d.Path, "/")
	if path == "" {
		path = defaultESCLPath
	}
	return "http://" + net.JoinHostPort(d.IP, strconv.Itoa(port)) + "/" + path + "/" + name
}

// 优先用设备原样上报的 ColorMode（比如 RGB48），没有才按规范给默认值。
func pickColorMode(in *inputCaps, key string) string {
	if in != nil {
		for _, m := range in.RawModes {
			if colorKey(m) == key {
				return m
			}
		}
	}
	return esclColorName(key)
}

// 精确匹配 → Document → 设备第一项。
func pickIntent(in *inputCaps, want string) string {
	if want == "" {
		want = IntentDocument
	}
	if in == nil || len(in.Intents) == 0 {
		return want
	}
	for _, v := range in.Intents {
		if strings.EqualFold(v, want) {
			return v
		}
	}
	for _, v := range in.Intents {
		if strings.EqualFold(v, IntentDocument) {
			return v
		}
	}
	return in.Intents[0]
}

func esclColorName(key string) string {
	switch key {
	case "gray":
		return "Grayscale8"
	case "bw":
		return "BlackAndWhite1"
	default:
		return "RGB24"
	}
}

// 调用处按字段名读，不会把版本当格式。
type scanSettings struct {
	Version   string
	Format    string
	ColorMode string
	Source    string // Platen / ADF
	Opts      Options
	Caps      *inputCaps // 用来夹边界；nil = 不限制
}

// 尺寸按物理尺寸 × dpi，超限则夹到边界。
func buildScanSettings(s scanSettings) []byte {
	u := paperUnit(s.Opts.Size)
	scale := float64(s.Opts.DPI) / 300.0
	pw, ph := clampToCaps(s.Caps, u, scale)

	adf := ""
	if s.Source == "ADF" {
		adf = "\n  <scan:ADFMode>Simplex</scan:ADFMode>"
	}
	intent := s.Opts.Intent
	if intent == "" {
		intent = IntentDocument
	}
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<scan:ScanSettings xmlns:scan="http://schemas.hp.com/imaging/escl/2011/05/03" xmlns:pwg="http://www.pwg.org/schemas/2010/12/sm">
  <pwg:Version>%s</pwg:Version>
  <scan:Intent>%s</scan:Intent>
  <pwg:ScanRegions>
    <pwg:ScanRegion>
      <pwg:Height>%d</pwg:Height>
      <pwg:Width>%d</pwg:Width>
      <pwg:XOffset>0</pwg:XOffset>
      <pwg:YOffset>0</pwg:YOffset>
    </pwg:ScanRegion>
  </pwg:ScanRegions>
  <pwg:DocumentFormat>%s</pwg:DocumentFormat>
  <scan:ColorMode>%s</scan:ColorMode>
  <scan:XResolution>%d</scan:XResolution>
  <scan:YResolution>%d</scan:YResolution>
  <scan:InputSource>%s</scan:InputSource>%s
</scan:ScanSettings>
`, s.Version, intent, ph, pw, s.Format, s.ColorMode, s.Opts.DPI, s.Opts.DPI, s.Source, adf)
	return []byte(xml)
}

// 把 300dpi 基准的纸面尺寸换算到目标 dpi，再夹进设备报的最大区域。
func clampToCaps(in *inputCaps, unit [2]int, scale float64) (w, h int) {
	w = int(float64(unit[0])*scale + 0.5)
	h = int(float64(unit[1])*scale + 0.5)
	if in == nil || in.MaxWidth <= 0 || in.MaxHeight <= 0 {
		return w, h
	}
	if mw := int(float64(in.MaxWidth) * scale); w > mw {
		w = mw
	}
	if mh := int(float64(in.MaxHeight) * scale); h > mh {
		h = mh
	}
	return w, h
}

// 任务 URL 在 Location 头里。
func createScanJob(ctx context.Context, d *DeviceInfo, settings []byte) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, jobTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.scanJobsURL(), bytes.NewReader(settings))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("Accept", "application/xml")
	resp, err := scannerHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("建扫描任务失败：%w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("扫描仪拒绝任务（HTTP %d），可能正在忙或盖板没关好", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("扫描仪没有返回任务地址")
	}
	return resolveURL(d.scanJobsURL(), loc), nil
}

func resolveURL(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// 刚建任务时设备常回 503，退避重试放在 waitForDocument 里。
func nextDocument(ctx context.Context, jobURL string, expect time.Duration, progress func(float64)) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
	start := time.Now()

	stop := estimateTicker(progress, func() float64 {
		return scanEstimate(time.Since(start), expect)
	})
	defer stop()

	resp, err := waitForDocument(ctx, strings.TrimRight(jobURL, "/")+"/NextDocument")
	if err != nil {
		return nil, "", err
	}
	if resp == nil {
		return nil, "", nil // 404/410：没有下一张了
	}
	// 必须在下载前收掉 ticker：一个迟到的估算值会让进度往回跳。
	stop()
	scan := scanEstimate(time.Since(start), expect)
	data, err := readDocument(resp, func(f float64) {
		if progress != nil {
			progress(scan + (1-scan)*f)
		}
	})
	if err != nil {
		return nil, "", fmt.Errorf("读取扫描页失败：%w", err)
	}
	return data, resp.Header.Get("Content-Type"), nil
}

// 返回 nil, nil 表示设备说没有下一张了。
func waitForDocument(ctx context.Context, pageURL string) (*http.Response, error) {
	delay := 250 * time.Millisecond
	// 复用同一个 timer：退避循环会转很多轮。
	wait := time.NewTimer(delay)
	defer wait.Stop()
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, err
		}
		resp, err := scannerHTTP.Do(req)
		if err == nil {
			switch resp.StatusCode {
			case http.StatusOK:
				return resp, nil
			case http.StatusNotFound, http.StatusGone:
				resp.Body.Close()
				return nil, nil
			case http.StatusServiceUnavailable, http.StatusConflict:
				resp.Body.Close() // 还没准备好，等一会儿再取
			default:
				resp.Body.Close()
				return nil, fmt.Errorf("取扫描页失败（HTTP %d）", resp.StatusCode)
			}
		}
		wait.Reset(delay)
		select {
		case <-ctx.Done():
			return nil, errors.New("等待扫描结果超时")
		case <-wait.C:
		}
		delay = min(2*delay, 2*time.Second)
	}
}

// stop 返回后不会再回调，下载阶段好接着推进度。
func estimateTicker(progress func(float64), est func() float64) (stop func()) {
	if progress == nil {
		return func() {}
	}
	tickDone := make(chan struct{})
	stopCh := make(chan struct{})
	var once sync.Once
	stop = func() {
		once.Do(func() { close(stopCh) })
		<-tickDone
	}
	go func() {
		defer close(tickDone)
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-stopCh:
				return
			case <-t.C:
				progress(est())
			}
		}
	}()
	return stop
}

// 进度最多每 100ms 报一次，别让一次下载推几百次界面重绘。
func readDocument(resp *http.Response, progress func(float64)) ([]byte, error) {
	defer resp.Body.Close()
	if progress == nil || resp.ContentLength <= 0 {
		return io.ReadAll(resp.Body)
	}
	total := float64(resp.ContentLength)
	var buf bytes.Buffer
	chunk := make([]byte, 64<<10)
	last := time.Now()
	for {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
			if now := time.Now(); now.Sub(last) >= 100*time.Millisecond {
				last = now
				progress(min(1, float64(buf.Len())/total))
			}
		}
		if err == io.EOF {
			progress(1)
			return buf.Bytes(), nil
		}
		if err != nil {
			return buf.Bytes(), err
		}
	}
}

// 收尾 DELETE 掉，释放扫描仪；失败无声——设备可能已经自己清了。
func cancelScanJob(jobURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, jobURL, nil)
	if err != nil {
		return
	}
	if resp, err := scannerHTTP.Do(req); err == nil {
		resp.Body.Close()
	}
}
