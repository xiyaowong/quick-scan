//go:build windows && amd64 && cgo

// ocrbridge 的 cgo 绑定：静态链 Rust staticlib，只搬运原始检测数据，筛选在 ocr.go。
package scanner

/*
#cgo LDFLAGS: -L${SRCDIR}/../../ocrbridge/target/x86_64-pc-windows-msvc/release -locrbridge -lntdll

#include <stdlib.h>
#include <stddef.h>

// 与 Rust 端 #[repr(C)] OcrLine 逐字段对应，顺序不能改。
typedef struct {
	unsigned char* text;
	size_t         text_len;
	float          score;
	float          top;
	float          left;
	float          width;
	float          height;
} ocr_line;

void*  ocr_init(const unsigned char* det, size_t det_len,
                const unsigned char* rec, size_t rec_len,
                const unsigned char* dict, size_t dict_len);
int    ocr_run(void* h, const unsigned char* png, size_t png_len,
               ocr_line** lines, size_t* n);
void   ocr_free_lines(ocr_line* p, size_t n);
void   ocr_free(void* h);
size_t ocr_last_error(unsigned char* buf, size_t buf_len);
*/
import "C"

import (
	"fmt"
	"path/filepath"
	"sync"
	"unsafe"

	"sscan/internal/platform"
)

// ocrLine 一行原始识别结果：文字、置信度、相对输入图的文字框几何。
type ocrLine struct {
	Text   string
	Score  float32
	Top    float32
	Left   float32
	Width  float32
	Height float32
}

var (
	ocrOnce    sync.Once
	ocrHandle  unsafe.Pointer
	ocrInitErr error
)

// ocrEngine 懒加载模型（首用时一次性）
func ocrEngine() (unsafe.Pointer, error) {
	ocrOnce.Do(func() {
		dir := platform.ModelDir()
		det := C.CString(filepath.Join(dir, "det.onnx"))
		defer C.free(unsafe.Pointer(det))
		rec := C.CString(filepath.Join(dir, "rec.onnx"))
		defer C.free(unsafe.Pointer(rec))
		dict := C.CString(filepath.Join(dir, "dict.txt"))
		defer C.free(unsafe.Pointer(dict))
		h := C.ocr_init((*C.uchar)(unsafe.Pointer(det)), C.size_t(len(filepath.Join(dir, "det.onnx"))),
			(*C.uchar)(unsafe.Pointer(rec)), C.size_t(len(filepath.Join(dir, "rec.onnx"))),
			(*C.uchar)(unsafe.Pointer(dict)), C.size_t(len(filepath.Join(dir, "dict.txt"))))
		if h == nil {
			ocrInitErr = fmt.Errorf("OCR 初始化失败: %s", ocrErrText())
			return
		}
		ocrHandle = h
	})
	return ocrHandle, ocrInitErr
}

// ocrRecognize 识别 PNG 字节，返回识别行
func ocrRecognize(png []byte) ([]ocrLine, error) {
	h, err := ocrEngine()
	if err != nil {
		return nil, err
	}
	if len(png) == 0 {
		return nil, fmt.Errorf("空图片")
	}
	var lines *C.ocr_line
	var n C.size_t
	rc := C.ocr_run(h, (*C.uchar)(unsafe.Pointer(&png[0])), C.size_t(len(png)),
		&lines, &n)
	if rc != 0 {
		return nil, fmt.Errorf("OCR 识别失败(%d): %s", rc, ocrErrText())
	}
	if lines == nil || n == 0 {
		return nil, nil
	}
	defer C.ocr_free_lines(lines, n)
	out := make([]ocrLine, int(n))
	lp := unsafe.Pointer(lines)
	for i := 0; i < int(n); i++ {
		cl := (*C.ocr_line)(unsafe.Add(lp, i*int(unsafe.Sizeof(*lines))))
		if cl.text != nil && cl.text_len > 0 {
			out[i] = ocrLine{
				Text:   C.GoStringN((*C.char)(unsafe.Pointer(cl.text)), C.int(cl.text_len)),
				Score:  float32(cl.score),
				Top:    float32(cl.top),
				Left:   float32(cl.left),
				Width:  float32(cl.width),
				Height: float32(cl.height),
			}
		}
	}
	return out, nil
}

// ocrErrText 取 Rust 侧最近一次错误文本
func ocrErrText() string {
	var buf [1024]C.uchar
	n := C.ocr_last_error(&buf[0], C.size_t(len(buf)))
	if n == 0 {
		return "未知错误"
	}
	return C.GoStringN((*C.char)(unsafe.Pointer(&buf[0])), C.int(n))
}
