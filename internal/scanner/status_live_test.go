package scanner

import (
	"context"
	"os"
	"testing"
)

// 真机状态联调：
//
//	SIMPLESCAN_LIVE=1 SIMPLESCAN_IP=192.168.3.111 just test -run TestLiveStatus -v
func TestLiveStatus(t *testing.T) {
	liveEnabled(t)
	ip := os.Getenv("SIMPLESCAN_IP")
	if ip == "" {
		t.Skip("设置 SIMPLESCAN_IP 才跑这台设备")
	}
	d := DeviceInfo{IP: ip, Port: 80, Path: "eSCL"}
	if err := Probe(context.Background(), &d); err != nil {
		t.Fatalf("probe: %v", err)
	}
	st, err := FetchStatus(context.Background(), &d)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	t.Logf("State=%q ADF=%q known=%v busy=%v stopped=%v", st.State, st.ADF, st.Known(), st.Busy(), st.Stopped())
	if !st.Known() {
		t.Fatal("设备没有上报任何状态")
	}
}
