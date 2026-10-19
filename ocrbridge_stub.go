//go:build !(windows && amd64 && cgo)

// ocrbridge 的占位实现：非 64 位 Windows / 未开 cgo 的构建
// （go vet、纯 Go 交叉检查等场景）。识别永远报"不可用"，编排层静默吞掉。
package main

import "fmt"

type ocrLine struct {
	Text   string
	Score  float32
	Top    float32
	Left   float32
	Width  float32
	Height float32
}

func ocrRecognize(png []byte) ([]ocrLine, error) {
	return nil, fmt.Errorf("OCR 仅在 64 位 Windows（cgo + rusto-rs）构建中可用")
}
