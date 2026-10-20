// eSCL 客户端。只用 IPv4；端口与路径来自 mDNS TXT，缺省 80 / "eSCL"。
package scanner

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultESCLPath = "eSCL"
	probeTimeout    = 6 * time.Second   // 读能力：设备在线就该秒回
	jobTimeout      = 15 * time.Second  // 建任务
	pageTimeout     = 120 * time.Second // 取一张：600dpi 冷启动也要等
	defaultVersion  = "2.63"
	DefaultIntent   = "Document"
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

// parseCapabilities 解析能力 XML；解析失败不算离线，只是选项全开。
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
	sort.Slice(in.Resolutions, func(i, j int) bool { return in.Resolutions[i] < in.Resolutions[j] })
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
		for _, x := range keys {
			if x == k {
				out = append(out, k)
				break
			}
		}
	}
	return out
}

func appendUnique(dst []string, vs ...string) []string {
	for _, v := range vs {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		dup := false
		for _, e := range dst {
			if e == v {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, v)
		}
	}
	return dst
}

func appendUniqueFloat(dst []float32, v float32) []float32 {
	for _, e := range dst {
		if e == v {
			return dst
		}
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
)

var paperUnits = map[string][2]int{
	PaperA4:     {2480, 3508},
	PaperA5:     {1748, 2480},
	PaperLetter: {2550, 3300},
}

// 允许横竖两个方向；Max 缺失 = 不限制。
func (in *inputCaps) supportsSize(s string) bool {
	if in == nil || in.MaxWidth <= 0 || in.MaxHeight <= 0 {
		return true
	}
	w, h := paperUnits[s][0], paperUnits[s][1]
	const slack = 8 // 各厂商对 A4 的换算差几个单位
	fit := func(a, b int) bool { return a <= in.MaxWidth+slack && b <= in.MaxHeight+slack }
	return fit(w, h) || fit(h, w)
}

func (in *inputCaps) supportsColor(key string) bool {
	if in == nil || len(in.Colors) == 0 {
		return true
	}
	for _, k := range in.Colors {
		if k == key {
			return true
		}
	}
	return false
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
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
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
		caps = &scannerCapabilities{Version: defaultVersion}
	}
	d.caps = caps
	d.Online = true
	if d.Name == "" && caps.MakeAndModel != "" {
		d.Name = caps.MakeAndModel
	}
	d.Colors = nil
	for _, c := range []string{ColorModeColor, ColorModeGray, ColorModeBW} {
		if caps.supportsColor(c) {
			d.Colors = append(d.Colors, c)
		}
	}
	d.Resolutions = caps.resolutions()
	d.Intents = caps.intents()
	d.Sizes = nil
	for _, s := range []string{PaperA4, PaperA5, PaperLetter} {
		if caps.supportsSize(s) {
			d.Sizes = append(d.Sizes, s)
		}
	}
	d.Feeder = caps.ADF != nil
	return nil
}

type Options struct {
	Color  string // color / gray / bw
	DPI    int
	Size   string // a4 / a5 / letter
	Feeder bool
	Intent string // eSCL Intent；空 = Document
}

// 失败时 DELETE 掉任务。
func Scan(ctx context.Context, d *DeviceInfo, opts Options, dir string) ([]string, error) {
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

	format := pickFormat(append(append([]string{}, in.Formats...), caps.Formats...))
	if format == "" {
		return nil, errors.New("设备不支持扫描成图片（只报了 PDF 等格式）")
	}
	colorMode := pickColorMode(in, opts.Color)
	opts.Intent = pickIntent(in, opts.Intent)
	version := caps.Version
	if version == "" {
		version = defaultVersion
	}

	jobURL, err := createScanJob(ctx, d, buildScanSettings(version, format, colorMode, source, opts, in))
	if err != nil {
		return nil, err
	}
	defer cancelScanJob(jobURL)

	ext := ".jpg"
	if format == "image/png" {
		ext = ".png"
	}
	var files []string
	for i := 1; ; i++ {
		data, ct, err := nextDocument(ctx, jobURL)
		if err != nil {
			return nil, err
		}
		if data == nil {
			break // 404：没有下一张了
		}
		if strings.Contains(ct, "pdf") {
			return nil, errors.New("设备返回了 PDF 格式，无法在界面里显示")
		}
		path := filepath.Join(dir, fmt.Sprintf("page_%d_%d%s", time.Now().UnixMilli(), i, ext))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return nil, fmt.Errorf("保存扫描页失败：%w", err)
		}
		files = append(files, path)
		if !opts.Feeder {
			break // 玻璃板一次一张
		}
	}
	if len(files) == 0 {
		return nil, errors.New("扫描仪没有返回任何页面（可能缺纸）")
	}
	return files, nil
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

// pickColorMode 优先用设备原样上报的 ColorMode（比如 RGB48），没有就按规范给默认值。
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

// pickIntent 把界面选的 Intent 落到设备支持的值上：精确匹配 → Document → 设备第一项。
func pickIntent(in *inputCaps, want string) string {
	if want == "" {
		want = DefaultIntent
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
		if strings.EqualFold(v, DefaultIntent) {
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

// buildScanSettings 生成 ScanSettings XML：尺寸按物理尺寸 × dpi，超限则夹到边界。
func buildScanSettings(version, format, colorMode, source string, opts Options, in *inputCaps) []byte {
	w, h := paperUnits[opts.Size][0], paperUnits[opts.Size][1]
	scale := float64(opts.DPI) / 300.0
	pw := int(float64(w)*scale + 0.5)
	ph := int(float64(h)*scale + 0.5)
	if in != nil && in.MaxWidth > 0 && in.MaxHeight > 0 {
		mw := int(float64(in.MaxWidth) * scale)
		mh := int(float64(in.MaxHeight) * scale)
		if pw > mw {
			pw = mw
		}
		if ph > mh {
			ph = mh
		}
	}
	adf := ""
	if source == "ADF" {
		adf = "\n  <scan:ADFMode>Simplex</scan:ADFMode>"
	}
	intent := opts.Intent
	if intent == "" {
		intent = DefaultIntent
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
`, version, intent, ph, pw, format, colorMode, opts.DPI, opts.DPI, source, adf)
	return []byte(xml)
}

// createScanJob POST 建任务，返回任务 URL（Location 头）。
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

// nextDocument 取下一张；刚建任务时的 503 按退避重试。
func nextDocument(ctx context.Context, jobURL string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, pageTimeout)
	defer cancel()
	pageURL := strings.TrimRight(jobURL, "/") + "/NextDocument"
	delay := 250 * time.Millisecond
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, "", err
		}
		resp, err := scannerHTTP.Do(req)
		if err == nil {
			switch resp.StatusCode {
			case http.StatusOK:
				data, rerr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if rerr != nil {
					return nil, "", fmt.Errorf("读取扫描页失败：%w", rerr)
				}
				return data, resp.Header.Get("Content-Type"), nil
			case http.StatusNotFound, http.StatusGone:
				resp.Body.Close()
				return nil, "", nil
			case http.StatusServiceUnavailable, http.StatusConflict:
				resp.Body.Close() // 还没准备好，等一会儿再取
			default:
				resp.Body.Close()
				return nil, "", fmt.Errorf("取扫描页失败（HTTP %d）", resp.StatusCode)
			}
		}
		select {
		case <-ctx.Done():
			return nil, "", errors.New("等待扫描结果超时")
		case <-time.After(delay):
		}
		if delay < 2*time.Second {
			delay *= 2
		}
	}
}

// cancelScanJob 任务收尾：DELETE 掉，释放扫描仪（失败无声——设备可能已经自己清了）。
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
