// 打印机状态：eSCL 的 ScannerStatus。连得上不等于能扫——也可能正忙、卡纸。
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

func (s Status) Busy() bool {
	return strings.EqualFold(s.State, "processing") || strings.EqualFold(s.ADF, adfProcessing)
}

func (s Status) Stopped() bool { return strings.EqualFold(s.State, "stopped") }

func (s Status) ADFJam() bool      { return s.adfIs(adfJam) }
func (s Status) ADFDoorOpen() bool { return s.adfIs(adfDoorOpen) }

// Known 没报状态时只能当"不知道"，不能当空闲。
func (s Status) Known() bool { return s.State != "" || s.ADF != "" }

func (s Status) adfIs(want string) bool { return strings.EqualFold(s.ADF, want) }

// FetchStatus 失败不代表离线，离线由能力探测判断。
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

// parseStatus 只认根节点的 State/AdfState：Jobs 里每条 JobInfo 的 JobState 不能混进来。
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
