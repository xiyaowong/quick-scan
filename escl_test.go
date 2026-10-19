package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

const capsSample = `<?xml version="1.0" encoding="UTF-8"?>
<scan:ScannerCapabilities xmlns:scan="http://schemas.hp.com/imaging/escl/2011/05/03" xmlns:pwg="http://www.pwg.org/schemas/2010/12/sm">
  <pwg:Version>2.63</pwg:Version>
  <pwg:MakeAndModel>HP Laser MFP 136nw</pwg:MakeAndModel>
  <scan:Platen>
    <scan:PlatenInputCaps>
      <scan:MinWidth>2480</scan:MinWidth>
      <scan:MaxWidth>2550</scan:MaxWidth>
      <scan:MinHeight>3508</scan:MinHeight>
      <scan:MaxHeight>4200</scan:MaxHeight>
      <scan:SettingProfiles>
        <scan:SettingProfile>
          <scan:ColorModes>
            <scan:ColorMode>RGB24</scan:ColorMode>
            <scan:ColorMode>Grayscale8</scan:ColorMode>
            <scan:ColorMode>BlackAndWhite1</scan:ColorMode>
          </scan:ColorModes>
          <scan:DocumentFormats>
            <pwg:DocumentFormat>image/jpeg</pwg:DocumentFormat>
            <pwg:DocumentFormat>application/pdf</pwg:DocumentFormat>
          </scan:DocumentFormats>
          <scan:SupportedResolutions>
            <scan:DiscreteResolutions>
              <scan:DiscreteResolution><scan:XResolution>300</scan:XResolution><scan:YResolution>300</scan:YResolution></scan:DiscreteResolution>
              <scan:DiscreteResolution><scan:XResolution>600</scan:XResolution><scan:YResolution>600</scan:YResolution></scan:DiscreteResolution>
            </scan:DiscreteResolutions>
          </scan:SupportedResolutions>
        </scan:SettingProfile>
      </scan:SettingProfiles>
      <scan:SupportedIntents>
        <scan:Intent>Document</scan:Intent>
        <scan:Intent>TextAndGraphic</scan:Intent>
        <scan:Intent>Photo</scan:Intent>
        <scan:Intent>Preview</scan:Intent>
      </scan:SupportedIntents>
    </scan:PlatenInputCaps>
  </scan:Platen>
  <scan:ADF>
    <scan:AdfSimplexInputCaps>
      <scan:MaxWidth>2550</scan:MaxWidth>
      <scan:MaxHeight>4200</scan:MaxHeight>
      <scan:ColorModes><scan:ColorMode>RGB24</scan:ColorMode></scan:ColorModes>
      <scan:DocumentFormats><pwg:DocumentFormat>image/jpeg</pwg:DocumentFormat></scan:DocumentFormats>
      <scan:SupportedResolutions>
        <scan:DiscreteResolutions>
          <scan:DiscreteResolution><scan:XResolution>300</scan:XResolution><scan:YResolution>300</scan:YResolution></scan:DiscreteResolution>
        </scan:DiscreteResolutions>
      </scan:SupportedResolutions>
    </scan:AdfSimplexInputCaps>
  </scan:ADF>
</scan:ScannerCapabilities>`

func TestParseCapabilities(t *testing.T) {
	caps, err := parseCapabilities([]byte(capsSample))
	if err != nil {
		t.Fatal(err)
	}
	if caps.Version != "2.63" || caps.MakeAndModel != "HP Laser MFP 136nw" {
		t.Fatalf("header = %q / %q", caps.Version, caps.MakeAndModel)
	}
	if caps.ADF == nil {
		t.Fatal("ADF not detected")
	}
	for _, c := range []ColorMode{ColorColor, ColorGray, ColorBW} {
		if !caps.supportsColor(c) {
			t.Errorf("color %v should be supported", c)
		}
	}
	for _, s := range []PaperSize{SizeA4, SizeA5, SizeLetter} {
		if !caps.supportsSize(s) {
			t.Errorf("size %v should be supported", s)
		}
	}
	if got := caps.resolutions(); len(got) != 2 || got[0] != 300 || got[1] != 600 {
		t.Errorf("resolutions = %v", got)
	}
	if got := pickFormat(caps.Formats); got != "image/jpeg" {
		t.Errorf("format = %q", got)
	}
	if got := caps.intents(); len(got) != 4 || got[0] != "Document" || got[3] != "Preview" {
		t.Errorf("intents = %v", got)
	}
}

func TestParseCapabilitiesRangeAndNarrowPlaten(t *testing.T) {
	const x = `<scan:ScannerCapabilities xmlns:scan="s" xmlns:pwg="p">
<pwg:Version>2.0</pwg:Version>
<scan:Platen><scan:PlatenInputCaps>
  <scan:MaxWidth>2480</scan:MaxWidth><scan:MaxHeight>3508</scan:MaxHeight>
  <scan:SupportedResolutions><scan:ResolutionRange>
    <scan:XResolutionRange><scan:Min>100</scan:Min><scan:Max>1200</scan:Max><scan:Step>1</scan:Step></scan:XResolutionRange>
  </scan:ResolutionRange></scan:SupportedResolutions>
</scan:PlatenInputCaps></scan:Platen>
</scan:ScannerCapabilities>`
	caps, err := parseCapabilities([]byte(x))
	if err != nil {
		t.Fatal(err)
	}
	if got := caps.resolutions(); len(got) != 2 || got[0] != 300 || got[1] != 600 {
		t.Errorf("range resolutions = %v", got)
	}
	if caps.supportsSize(SizeLetter) {
		t.Error("Letter should not fit a 2480x3508 area")
	}
	if !caps.supportsSize(SizeA4) {
		t.Error("A4 should fit")
	}
	if caps.supportsColor(ColorColor) != true || caps.supportsColor(ColorGray) != true {
		t.Error("missing color caps should not restrict choices")
	}
}

func TestBuildScanSettings(t *testing.T) {
	in := &inputCaps{MaxWidth: 2550, MaxHeight: 4200}
	got := string(buildScanSettings("2.63", "image/jpeg", "RGB24", "Platen",
		ScanOptions{Color: ColorColor, DPI: 300, Size: SizeA4}, in))
	for _, want := range []string{
		"<pwg:Version>2.63</pwg:Version>",
		"<pwg:Width>2480</pwg:Width>",
		"<pwg:Height>3508</pwg:Height>",
		"<pwg:DocumentFormat>image/jpeg</pwg:DocumentFormat>",
		"<scan:ColorMode>RGB24</scan:ColorMode>",
		"<scan:XResolution>300</scan:XResolution>",
		"<scan:InputSource>Platen</scan:InputSource>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("settings missing %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "ADFMode") {
		t.Error("platen settings must not contain ADFMode")
	}

	got = string(buildScanSettings("2.63", "image/jpeg", "RGB24", "ADF",
		ScanOptions{Color: ColorColor, DPI: 600, Size: SizeA4, Feeder: true, Intent: "Photo"}, in))
	if !strings.Contains(got, "<scan:InputSource>ADF</scan:InputSource>") ||
		!strings.Contains(got, "<scan:ADFMode>Simplex</scan:ADFMode>") {
		t.Errorf("ADF settings wrong\n%s", got)
	}
	if !strings.Contains(got, "<scan:Intent>Photo</scan:Intent>") {
		t.Errorf("intent not sent\n%s", got)
	}
	// 600dpi 的 A4 高度 7016 超过设备上限 4200*2=8400? 没有，宽度 4960 ≤ 5100；
	// 这里验证的是尺寸随 dpi 翻倍
	if !strings.Contains(got, "<pwg:Width>4960</pwg:Width>") {
		t.Errorf("600dpi width wrong\n%s", got)
	}
}

func TestDisplayName(t *testing.T) {
	d := DeviceInfo{Name: "HP Laser MFP 136nw", IP: "192.168.1.20"}
	if got := d.DisplayName(); got != "HP Laser MFP 136nw (192.168.1.20)" {
		t.Fatalf("display = %q", got)
	}
	if got := (&DeviceInfo{IP: "10.0.0.2"}).DisplayName(); got != "10.0.0.2 (10.0.0.2)" {
		t.Fatalf("fallback display = %q", got)
	}
}

func TestBrowserDevices(t *testing.T) {
	b := &mdnsBrowser{
		records: map[string]*serviceRecord{},
		addrs:   map[string]string{},
	}
	rec := b.record(`HP Laser MFP 116w (5D:DF:A6)._uscan._tcp.local.`)
	rec.hasPTR, rec.hasSRV = true, true
	rec.port = 80
	rec.host = "HP30138B5DDFA6.local."
	rec.txt["ty"] = "HP Laser MFP 116w"
	rec.txt["rs"] = "eSCL"
	b.addrs["hp30138b5ddfa6.local."] = "192.168.3.111"

	devs := b.devices()
	if len(devs) != 1 {
		t.Fatalf("devices = %+v", devs)
	}
	d := devs[0]
	if d.Name != "HP Laser MFP 116w" || d.IP != "192.168.3.111" || d.Port != 80 || d.Path != "eSCL" {
		t.Fatalf("device = %+v", d)
	}
	if got := d.DisplayName(); got != "HP Laser MFP 116w (192.168.3.111)" {
		t.Fatalf("display = %q", got)
	}
}

func TestShortInstanceName(t *testing.T) {
	got := shortInstanceName(`HP\ Laser\ MFP\ 116w\ \(5D:DF:A6\)._uscan._tcp.local.`)
	if got != "HP Laser MFP 116w (5D:DF:A6)" {
		t.Fatalf("short name = %q", got)
	}
}

func TestParseTXT(t *testing.T) {
	txt := parseTXT([]string{"ty=HP Laser MFP 116w", "rs=eSCL", "duplex=F"})
	if txt["ty"] != "HP Laser MFP 116w" || txt["rs"] != "eSCL" {
		t.Fatalf("txt = %v", txt)
	}
}

func TestPickFormat(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"application/pdf", "image/jpeg"}, "image/jpeg"},
		{[]string{"image/png", "image/jpeg"}, "image/jpeg"},
		{[]string{"image/png"}, "image/png"},
		{[]string{"application/pdf"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := pickFormat(c.in); got != c.want {
			t.Errorf("pickFormat(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// fakeScanner 起一个最小 eSCL 服务：能力 + 任务 + N 张图 + 404。
type fakeScanner struct {
	srv        *httptest.Server
	pages      int
	nextCalls  int32
	deleted    int32
	lastCreate string
}

func newFakeScanner(t *testing.T, pages int) *fakeScanner {
	t.Helper()
	fs := &fakeScanner{pages: pages}
	mux := http.NewServeMux()
	mux.HandleFunc("/eSCL/ScannerCapabilities", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, capsSample)
	})
	mux.HandleFunc("/eSCL/ScanJobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		var buf bytes.Buffer
		buf.ReadFrom(r.Body)
		fs.lastCreate = buf.String()
		w.Header().Set("Location", "/eSCL/ScanJobs/42")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("/eSCL/ScanJobs/42/NextDocument", func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&fs.nextCalls, 1))
		if n > fs.pages {
			http.Error(w, "no more", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(tinyJPEG(t))
	})
	mux.HandleFunc("/eSCL/ScanJobs/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			atomic.AddInt32(&fs.deleted, 1)
		}
		w.WriteHeader(http.StatusOK)
	})
	fs.srv = httptest.NewServer(mux)
	t.Cleanup(fs.srv.Close)
	return fs
}

func (fs *fakeScanner) device() DeviceInfo {
	u, _ := url.Parse(fs.srv.URL)
	host, portStr, _ := net.SplitHostPort(u.Host)
	var port int
	fmt.Sscan(portStr, &port)
	return DeviceInfo{Name: "HP Laser MFP 136nw", IP: host, Port: port, Path: "eSCL"}
}

func tinyJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestProbeAndScanPlaten(t *testing.T) {
	fs := newFakeScanner(t, 1)
	d := fs.device()
	if err := probeDevice(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	if !d.Online || !d.Feeder || d.Name != "HP Laser MFP 136nw" {
		t.Fatalf("probe = %+v", d)
	}
	if len(d.Colors) != 3 || len(d.Sizes) != 3 || len(d.Resolutions) != 2 || len(d.Intents) != 4 {
		t.Fatalf("caps not applied: %+v", d)
	}

	dir := t.TempDir()
	files, err := scanDocuments(context.Background(), &d,
		ScanOptions{Color: ColorColor, DPI: 300, Size: SizeA4, Intent: "Photo"}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	if _, err := os.Stat(files[0]); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fs.lastCreate, "<scan:InputSource>Platen</scan:InputSource>") ||
		!strings.Contains(fs.lastCreate, "<scan:Intent>Photo</scan:Intent>") {
		t.Errorf("settings posted = %s", fs.lastCreate)
	}
	if atomic.LoadInt32(&fs.deleted) != 1 {
		t.Error("scan job was not deleted")
	}
}

func TestScanADFMultiplePages(t *testing.T) {
	fs := newFakeScanner(t, 3)
	d := fs.device()
	if err := probeDevice(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files, err := scanDocuments(context.Background(), &d,
		ScanOptions{Color: ColorGray, DPI: 300, Size: SizeA4, Feeder: true}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("ADF pages = %d, want 3", len(files))
	}
	for _, f := range files {
		if filepath.Ext(f) != ".jpg" {
			t.Errorf("unexpected extension: %s", f)
		}
	}
	if !strings.Contains(fs.lastCreate, "<scan:ColorMode>Grayscale8</scan:ColorMode>") {
		t.Errorf("color mode not sent: %s", fs.lastCreate)
	}
	if !strings.Contains(fs.lastCreate, "<scan:ADFMode>Simplex</scan:ADFMode>") {
		t.Errorf("ADF mode not sent: %s", fs.lastCreate)
	}
}

func TestDPIChoicesAndIntentFallback(t *testing.T) {
	d := &DeviceInfo{
		Resolutions: []float32{300, 100, 200, 200},
		Intents:     []string{"Document", "Photo"},
	}
	if got := dpiChoices(d); len(got) != 3 || got[0] != 100 || got[1] != 200 || got[2] != 300 {
		t.Fatalf("dpiChoices = %v", got)
	}
	if !d.dpiOK(200) || d.dpiOK(600) || d.defaultDPI() != 300 {
		t.Fatalf("dpi helpers wrong: ok200=%v ok600=%v default=%d", d.dpiOK(200), d.dpiOK(600), d.defaultDPI())
	}
	if !d.intentOK("photo") || d.intentOK("Preview") || d.defaultIntent() != "Document" {
		t.Fatalf("intent helpers wrong: photo=%v preview=%v default=%q",
			d.intentOK("photo"), d.intentOK("Preview"), d.defaultIntent())
	}

	// 300 不在列表里：取最接近的一档
	near := &DeviceInfo{Resolutions: []float32{150, 600}}
	if got := near.defaultDPI(); got != 150 {
		t.Errorf("nearest dpi = %d, want 150", got)
	}

	// 没上报能力：保持 300/600 兜底，不限制
	if got := dpiChoices(&DeviceInfo{}); len(got) != 2 || got[0] != 300 || got[1] != 600 {
		t.Errorf("fallback dpis = %v", got)
	}
	if got := intentChoices(&DeviceInfo{}); len(got) != 1 || got[0] != defaultIntent {
		t.Errorf("fallback intents = %v", got)
	}

	in := &inputCaps{Intents: []string{"Photo", "Document"}}
	if got := pickIntent(in, "photo"); got != "Photo" {
		t.Errorf("pickIntent exact = %q", got)
	}
	if got := pickIntent(in, "TextAndGraphic"); got != "Document" {
		t.Errorf("pickIntent fallback = %q", got)
	}
	if got := pickIntent(nil, ""); got != defaultIntent {
		t.Errorf("pickIntent default = %q", got)
	}
}

func TestProbeOffline(t *testing.T) {
	d := DeviceInfo{Name: "gone", IP: "192.0.2.1", Port: 9, Path: "eSCL"}
	ctx, cancel := context.WithTimeout(context.Background(), 200_000_000) // 200ms
	defer cancel()
	if err := probeDevice(ctx, &d); err == nil {
		t.Fatal("expected failure")
	}
	if d.Online {
		t.Fatal("offline device marked online")
	}
}
