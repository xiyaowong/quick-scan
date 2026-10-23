package scanner

import (
	"context"
	"errors"
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
	// 进纸口没纸只是设备事实，不构成"忙"。
	if st.Busy() || st.Stopped() || st.ADFJam() || !st.Known() {
		t.Fatalf("idle 状态判断错了：%+v", st)
	}

	// 设备什么都不报：不能当成空闲。
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

// 状态和发现走同一个端点前缀：Port/Path 要跟着设备走。
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

	// 非 200 要报错，不能拿假状态糊弄上层。
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})
	_, err = FetchStatus(context.Background(), &d)
	if err == nil {
		t.Fatal("HTTP 500 应该报错")
	}
	// 设备回了话：上层不能据此判脱机。
	if errors.Is(err, ErrUnreachable) {
		t.Fatalf("HTTP 报错被当成了连不上：%v", err)
	}
}

// 设备根本没应答（关机、脱机）要和端点报错分开：上层靠它决定是否标记离线。
func TestFetchStatusUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	u, _ := url.Parse(srv.URL)
	host, portStr, _ := net.SplitHostPort(u.Host)
	port, _ := strconv.Atoi(portStr)
	srv.Close() // 端口随即关闭：连接被拒绝

	d := DeviceInfo{IP: host, Port: port, Path: "eSCL"}
	_, err := FetchStatus(context.Background(), &d)
	if !errors.Is(err, ErrUnreachable) {
		t.Fatalf("err = %v，want ErrUnreachable", err)
	}
	// 底层网络错误也要留在链上：排查时不能只剩一句“设备连不上”。
	if _, ok := errors.AsType[*url.Error](err); !ok {
		t.Fatalf("底层网络错误没保留：%v", err)
	}
}
