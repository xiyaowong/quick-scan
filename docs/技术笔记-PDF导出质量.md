# 技术笔记 · PDF 导出质量（printpdf 0.12.8 的坑）

> 触发问题：用户实测"导出的图片还可以，pdf 糊得没边了"。JPG 导出正常、PDF 糊，
> 说明问题在 PDF 这一路，不在扫描/旋转。

## 根因：`PdfSaveOptions::default()` 自带"图像优化"，会偷偷把整页图缩小

`printpdf-0.12.8/src/serialize.rs:58` 起，`PdfSaveOptions::default()` 里
`image_optimization: Some(ImageOptimizationOptions::default())`，而后者默认值是
（`image_types.rs:40-75`）：

| 字段 | 默认值 | 后果 |
|---|---|---|
| `quality` | `0.85` | DCT（JPEG）重压一次 |
| `max_image_size` | `Some("2MB")` | **超限就缩图** |
| `format` | `Auto` | 彩色 → `DCTDecode`；灰度 → `LZWDecode`（实际走 `FlateDecode`） |
| `auto_optimize` | `true` | 去 alpha、彩色转灰度（无损） |

关键在 `image.rs:625-634` 的 `optimize()`：

```rust
if let Some(max_size) = options.max_image_size.as_deref().and_then(|s| parse_size_string(s).ok()) {
    if self.estimate_size_bytes() > max_size {   // estimate_size_bytes = 宽*高*bpp（未压缩估算）
        self.resize_to_fit_size(max_size)?;      // ← 最近邻缩图（image.rs:1095 起）
    }
}
```

A4 300dpi 彩色一页 = 2481×3508×24bit ≈ **24.9 MB** 的"未压缩估算"，而阈值是 2MB：
`scale = sqrt(2/24.9) ≈ 0.283`，图片被最近邻缩到约 702×993，再按 300dpi 名义尺寸铺到整页——
等于把 1/3.5 分辨率的图放大 3.5 倍画出去，**糊就是这么来的**（最近邻缩图还叠加了锯齿）。

次要问题：我们原来的实现先把 BMP → JPEG(q88)，再 `RawImage::decode_from_bytes` 解回来，
printpdf 又压一遍 JPEG(q85)。一次有损编码被做了两遍，而第一遍完全是白做的。

## 修法（`app/main/src/export.rs::export_pdf`）

1. **不复用 JPG 中间产物**：`image::open(BMP)` → `apply_rotation` → `to_rgb8()`，
   直接手搓 `RawImage { pixels: RawImageData::U8(..), data_format: RawImageFormat::RGB8, .. }`
   喂给 `doc.add_image()`。全程只压一次。
2. **自己给 `PdfSaveOptions` 控参**，别吃默认值：

```rust
let save = PdfSaveOptions {
    image_optimization: Some(ImageOptimizationOptions {
        quality: Some(0.9),        // 唯一一次 JPEG，质量给足
        max_image_size: None,      // ★ 千万置 None，否则 2MB 默认值会缩图
        format: Some(ImageCompression::Jpeg),
        ..Default::default()       // auto_optimize 保留：灰度稿自动单通道，无画质损失
    }),
    ..Default::default()
};
doc.with_pages(pdf_pages).save(&save, &mut Vec::new())
```

### 顺带记两点

- 不能图省事把 `image_optimization` 设成 `None`：`image.rs:1291` 是
  `if let Some(opts) = options { ... }`，options 为 None 时 `compressed_pixels` 保持原始
  像素且**不设 Filter**，每页 25MB 裸像素直写 PDF，文件会大得离谱。要无损就该显式给
  `format: Some(ImageCompression::Flate)`。
- `XObjectTransform { dpi: Some(300.0) }` 配 `Mm(px * 25.4 / dpi)` 的页尺寸是对的
  （`xobject.rs:614`：`let dpi = self.dpi.unwrap_or(300.0)`，缩放 = `w.into_pt(dpi)`），
  与本次糊图无关，别改。
