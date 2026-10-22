package scanner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

const statusSample = `<?xml version="1.0" encoding="UTF-8"?>
<scan:ScannerStatus xmlns:scan="http://schemas.hp.com/imaging/escl/2011/05/03" xmlns:pwg="http://www.pwg.org/schemas/2010/12/sm">
<pwg:Version>2.5</pwg:Version>
<pwg:State>Processing</pwg:State>
<scan:AdfState>ScannerAdfJam</scan:AdfState>
<scan:Jobs>
  <scan:JobInfo>
    <scan:JobUri>/eSCL/ScanJobs/1</scan:JobUri>
    <pwg:JobState>Canceled</pwg:JobState>
    <pwg:JobStateReasons><pwg:JobStateReason>JobCanceledByUser</pwg:JobStateReason></pwg:JobStateReasons>
  </scan:JobInfo>
</scan:Jobs>
</scan:ScannerStatus>`

// Jobs 里每条的 JobState 不能混进根节点的 State。
func TestParseStatus(t *testing.T) {
	st, err := parseStatus([]byte(statusSample))
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "processing" || st.ADF != "ScannerAdfJam" {
		t.Fatalf("status = %+v", st)
	}
	if !st.Known() || !st.Busy() || !st.ADFJam() {
		t.Fatalf("状态判断不对：%+v busy=%v jam=%v", st, st.Busy(), st.ADFJam())
	}
	if st.Stopped() || st.ADFDoorOpen() {
		t.Fatalf("误判了别的状态：%+v", st)
	}
}

func TestStatusIdleAndEmpty(t *testing.T) {
	st, err := parseStatus([]byte(`<scan:ScannerStatus xmlns:scan="s" xmlns:pwg="p">
<pwg:State>Idle</pwg:State><scan:AdfState>ScannerAdfEmpty</scan:AdfState></scan:ScannerStatus>`))
	if err != nil {
		t.Fatal(err)
	}
	// 进纸口没纸只是设备的一个事实，不构成"忙"；界面也不该因此拦扫描。
	if st.Busy() || st.Stopped() || st.ADFJam() || !st.Known() {
		t.Fatalf("idle 状态判断错了：%+v", st)
	}

	// 设备什么都不报：Known 必须是 false，不能被当成空闲。
	empty, err := parseStatus([]byte(`<scan:ScannerStatus xmlns:scan="s"/>`))
	if err != nil {
		t.Fatal(err)
	}
	if empty.Known() || empty.Busy() {
		t.Fatalf("空状态 = %+v", empty)
	}
}

func TestStatusStoppedAndDoor(t *testing.T) {
	for _, raw := range []string{
		`<scan:ScannerStatus xmlns:scan="s" xmlns:pwg="p"><pwg:State>Stopped</pwg:State></scan:ScannerStatus>`,
		`<scan:ScannerStatus xmlns:scan="s" xmlns:pwg="p"><pwg:State>  STOPPED </pwg:State></scan:ScannerStatus>`,
	} {
		st, err := parseStatus([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		if !st.Stopped() {
			t.Fatalf("%s → %+v", raw, st)
		}
	}
	st := Status{ADF: "ScannerAdfDoorOpen"}
	if !st.ADFDoorOpen() || st.ADFJam() {
		t.Fatalf("门开判断错了：%+v", st)
	}
}

// 状态和发现走同一个端点前缀：Port/Path 都要跟着设备走。
func TestFetchStatusUsesDeviceEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<scan:ScannerStatus xmlns:scan="s" xmlns:pwg="p"><pwg:State>Idle</pwg:State></scan:ScannerStatus>`)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	host, portStr, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portStr)
	d := DeviceInfo{IP: host, Port: port, Path: "/custom/"}

	st, err := FetchStatus(context.Background(), &d)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Known() || st.State != "idle" {
		t.Fatalf("status = %+v", st)
	}
	if gotPath != "/custom/ScannerStatus" {
		t.Fatalf("path = %q", gotPath)
	}

	// 非 200：报错，不能拿一个假状态糊弄上层。
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})
	if _, err := FetchStatus(context.Background(), &d); err == nil {
		t.Fatal("HTTP 500 应该报错")
	}
}
