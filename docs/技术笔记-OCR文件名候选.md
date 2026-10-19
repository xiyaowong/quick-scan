# 技术笔记：OCR 文件名候选（rusto-rs）

> 2026-10-15 调研定稿。目标：扫描后自动从首页识别标题性文字（项目名/合同名/报告名），
> 在导出对话框里提供 ~3 个文件名候选项，点击即换名。默认名称不变，导出流程不变。

## 选型结论：rusto-rs

- crate：`rusto-rs = "0.3"`（库名 `rusto`，v0.3.1，2026-10-10 发布）
- 本质：PaddleOCR 模型 + RTen 纯 Rust 推理引擎（0.3.x 起，0.2.x 的 MNN 后端已废弃）
- **纯 Rust，无 C/C++ 依赖**，Windows MSVC 64 位 `cargo build` 直接过（无 CMake/vcpkg）
- **中文原生支持**：PP-OCRv6 统一模型（rec + dict）覆盖 50+ 语言，含简体/繁体中文，
  中英混排无需换模型
- 性能（官方基准 PP-OCRv6 Tiny，A4 ~150dpi）：桌面 CPU 合计 ~80ms；换算我们 A4 300dpi
  （内部缩到 ~960-1600px 再推理）预估 0.2-0.5s/页，峰值内存 ~100MB → 必须后台线程

## 模型三件套（PP-OCRv6 Tiny，共 ~10MB）

来源：ModelScope RapidAI/RapidOCR（GitHub release 挂的是 0.2.x 的 .mnn，**不能用**）：

- 检测：`https://www.modelscope.cn/api/v1/models/RapidAI/RapidOCR/repo?Revision=master&FilePath=onnx%2FPP-OCRv6%2Fdet%2FPP-OCRv6_det_tiny.onnx`
- 识别：`...FilePath=onnx%2FPP-OCRv6%2Frec%2FPP-OCRv6_rec_tiny.onnx`
- 字典：`...FilePath=paddle%2FPP-OCRv6%2Frec%2FPP-OCRv6_rec_tiny%2Fppocrv6_tiny_dict.txt`

## 用法（关键 API）

```rust
use rusto::{InitializeConfig, RustO, ImageSource, OcrRunOptions, OutputGranularity, DetectTextResult};

let mut ocr = RustO::initialize(InitializeConfig::ppv6(det, rec, dict))?;
let mut opts = OcrRunOptions::default();
opts.output = OutputGranularity::Lines;   // 行粒度，取标题正合适
opts.text_score = Some(0.5);
opts.max_side_len = Some(1600.0);
let result = ocr.detect_text(&ImageSource::Bytes(png_bytes), &opts)?;
if let DetectTextResult::Structured(items) = result {
    for it in items { /* it.text, it.score, it.frame(top/left/w/h) */ }
}
```

## 落地要点（踩坑预防）

1. **BMP 坑**：rusto 的 image 依赖只开了 png+jpeg feature，**不吃 BMP**。
   用主程序已有的 `image`（带 bmp）解码 → 转 PNG 字节 → `ImageSource::Bytes`。
   顺带可缩放/裁剪（只送页面上部区域，提速一个量级）。
2. **`RustO` 实例不可跨线程共享**（持 session 状态）：后台任务里建、用完丢，
   或工作池每线程一个。我们每次扫描一张、低频，直接每次新建即可。
3. 模型文件必须随程序分发：单文件目标下走 `go:embed` 内嵌 + `%TEMP%` 释放
   （`assets.go`），rusto 收路径，释放后把路径传给桥接层即可。
4. 只取标题：`Lines` 粒度 + 按 `frame.top` 排序取最靠前的几条大字号行，
   过滤纯数字/日期/短于 4 字的行。
5. OCR 绝不能在 UI 线程跑（gpui）：走 `cx.background_executor()`，
   结果回主线程写状态，与灯箱解码同一套路。

## 备选（否决理由）

- tesseract 绑定：要 C++ 工具链，中文精度一般
- PaddleOCR native 绑定（ort）：要 ONNX Runtime native dll；rusto 本质就是它的纯 Rust 版
- Windows.Media.Ocr（WinRT）：零模型但要用户装中文语言包，交付不可控
