// 打印机状态：eSCL 的 ScannerStatus。和“连不连得上”是两回事——连上了也可能正忙、
// 卡纸、进纸口没关好，这些要能在用户点扫描之前就看见。
package scanner

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const statusTimeout = 4 * time.Second

// eSCL 规范里的 ADF 状态取值。
const (
	adfProcessing = "ScannerAdfProcessing"
	adfJam        = "ScannerAdfJam"
	adfDoorOpen   = "ScannerAdfDoorOpen"
)

// Status 设备自己报的当前状态。
type Status struct {
	State string // pwg:State：idle / processing / stopped / testing；空 = 没上报
	ADF   string // scan:AdfState 原样值
}

// Busy 别的任务正占着扫描仪。
func (s Status) Busy() bool {
	return strings.EqualFold(s.State, "processing") || strings.EqualFold(s.ADF, adfProcessing)
}

// Stopped 设备停住了：常见于卡纸、盖子没关、硬件报错。
func (s Status) Stopped() bool { return strings.EqualFold(s.State, "stopped") }

func (s Status) ADFJam() bool      { return s.adfIs(adfJam) }
func (s Status) ADFDoorOpen() bool { return s.adfIs(adfDoorOpen) }

// Known 设备确实报了状态；没报时只能当"不知道"，不能当成空闲。
func (s Status) Known() bool { return s.State != "" || s.ADF != "" }

func (s Status) adfIs(want string) bool { return strings.EqualFold(s.ADF, want) }

// FetchStatus 读一次设备状态。失败不代表设备离线——离线由能力探测判断。
func FetchStatus(ctx context.Context, d *DeviceInfo) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.statusURL(), nil)
	if err != nil {
		return Status{}, err
	}
	req.Header.Set("Accept", "application/xml, text/xml")
	resp, err := scannerHTTP.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("读不到设备状态：%w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return Status{}, fmt.Errorf("设备状态返回 HTTP %d", resp.StatusCode)
	}
	return parseStatus(body)
}

// parseStatus 只取根节点下的 State 和 AdfState：Jobs 里每条的 JobState 不能混进来。
func parseStatus(data []byte) (Status, error) {
	var raw struct {
		State string `xml:"State"`
		ADF   string `xml:"AdfState"`
	}
	if err := xml.Unmarshal(data, &raw); err != nil {
		return Status{}, err
	}
	return Status{
		State: strings.ToLower(strings.TrimSpace(raw.State)),
		ADF:   strings.TrimSpace(raw.ADF),
	}, nil
}

func (d *DeviceInfo) statusURL() string {
	return d.resourceURL("ScannerStatus")
}
