# 技术笔记：OCR 文件名候选

> 扫描首页后识别标题性文字，在导出对话框里给约 3 个文件名候选。
> 默认文件名与导出流程不变。

## 选型：rusto-rs

- `rusto-rs = "0.3"`：PaddleOCR 模型 + RTen 纯 Rust 推理，无 C/C++ 依赖。
- PP-OCRv6 Tiny 统一模型，中英文混排无需换模型。
- A4 300dpi 缩到约 1600px 后推理，单页约 0.2–0.5 秒，峰值内存约 100MB，必须后台跑。

## 模型三件套（约 10MB）

来自 ModelScope `RapidAI/RapidOCR`，`onnx/PP-OCRv6/` 下的 det、rec 和
`paddle/PP-OCRv6/.../ppocrv6_tiny_dict.txt`。GitHub release 里的 `.mnn` 是 0.2.x，不能用。

## 用法

```rust
let mut ocr = RustO::initialize(InitializeConfig::ppv6(det, rec, dict))?;
let mut opts = OcrRunOptions::default();
opts.output = OutputGranularity::Lines;
opts.max_side_len = Some(1600.0);
ocr.detect_text(&ImageSource::Bytes(png_bytes), &opts)?;
```

## 落地要点

1. rusto 只开了 PNG/JPEG feature，不吃 BMP。先用主程序的 `image` 解码，
   转 PNG 字节再送；也可只送页面上部并缩放，提速明显。
2. `RustO` 不能跨线程共享。每次扫描新建一个即可。
3. 模型用 `go:embed` 内嵌，运行时释放到 `%TEMP%`，把路径传给桥接层。
4. 取标题：`Lines` 粒度，按 `frame.top` 取最靠前的几条大字号行，
   过滤纯数字、日期和短于 4 字的行。
5. OCR 不在 UI 线程跑；结果回主线程再更新状态。
