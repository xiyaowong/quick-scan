//! ocrbridge —— rusto-rs 的 C ABI 薄桥，给 Go 主程序静态链入（仅 64 位）。
//!
//! 设计（沿用调研结论 + spike 验证过的用法）：
//! - 内存所有权全在 Rust：Go 拿到的指针只能用 ocr_free_* 还回来
//! - panic 一律 catch_unwind + abort 码，绝不穿过 FFI 边界
//! - 图片传 PNG 字节（rusto 的 ImageSource::Bytes 内部走 image 解码）
//! - 错误经 ocr_last_error 线程局部分隔，Go 侧读到再转成 Go error

use std::ffi::c_void;
use std::path::PathBuf;

use rusto::{DetectTextResult, ImageSource, InitializeConfig, OcrRunOptions, OutputGranularity, RustO};

/// OcrLine 一行识别结果。text 指向 Rust 分配的 UTF-8（含 NUL 结尾），
/// 用 ocr_free_lines 整批释放。
#[repr(C)]
pub struct OcrLine {
    pub text: *mut u8,
    pub text_len: usize,
    pub score: f32,
    pub top: f32,
    pub left: f32,
    pub width: f32,
    /// 文字框高度（原图裁切后的像素）：标题字号大，这是找标题最强的信号
    pub height: f32,
}

struct Bridge {
    ocr: RustO,
}

thread_local! {
    static LAST_ERROR: std::cell::RefCell<Option<String>> = const { std::cell::RefCell::new(None) };
}

fn set_err(msg: String) {
    LAST_ERROR.with(|e| *e.borrow_mut() = Some(msg));
}

/// ocr_init 加载三件套模型。det/rec/dict 是 UTF-8 路径。失败返回 NULL，
/// 细节用 ocr_last_error 取。
#[no_mangle]
pub extern "C" fn ocr_init(det: *const u8, det_len: usize, rec: *const u8, rec_len: usize,
                           dict: *const u8, dict_len: usize) -> *mut c_void {
    let det = match str_arg(det, det_len) { Some(s) => s, None => return std::ptr::null_mut() };
    let rec = match str_arg(rec, rec_len) { Some(s) => s, None => return std::ptr::null_mut() };
    let dict = match str_arg(dict, dict_len) { Some(s) => s, None => return std::ptr::null_mut() };

    let cfg = InitializeConfig::ppv6(PathBuf::from(det), PathBuf::from(rec), PathBuf::from(dict));
    match RustO::initialize(cfg) {
        Ok(ocr) => Box::into_raw(Box::new(Bridge { ocr })) as *mut c_void,
        Err(e) => {
            set_err(format!("模型加载失败: {e}"));
            std::ptr::null_mut()
        }
    }
}

/// ocr_run 识别一张 PNG。lines 由调用方传入 *mut *mut OcrLine、n 传 *mut usize，
/// 成功时指向 Rust 分配的数组（用 ocr_free_lines 释放）。返回 0 成功。
#[no_mangle]
pub extern "C" fn ocr_run(h: *mut c_void, png: *const u8, png_len: usize,
                          lines: *mut *mut OcrLine, n: *mut usize) -> i32 {
    let bridge: &mut Bridge = match unsafe { (h as *mut Bridge).as_mut() } {
        Some(b) => b,
        None => {
            set_err("无效句柄".into());
            return 1
        }
    };
    let Some(png) = bytes_arg(png, png_len) else { return 1 };

    // catch_unwind：release 是 panic=abort，这里主要防 debug 构建穿边界
    let res = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        let mut opts = OcrRunOptions::default();
        opts.output = OutputGranularity::Lines;
        // text_score 是 rusto 内部的硬过滤（低于阈值的行直接丢，Go 侧看不到），
        // 传 0 关掉它：候选的取舍/排序全部交给 Go（ocr.go），便于随时调。
        opts.text_score = Some(0.0);
        bridge.ocr.detect_text(&ImageSource::Bytes(png.to_vec()), &opts)
    }));
    let result = match res {
        Ok(r) => match r {
            Ok(r) => r,
            Err(e) => {
                set_err(format!("识别失败: {e}"));
                return 2
            }
        },
        Err(_) => {
            set_err("识别过程 panic".into());
            return 2
        }
    };
    match result {
        DetectTextResult::Structured(items) => unsafe {
            let count = items.len();
            let arr = libc_alloc_lines(count);
            for (i, it) in items.into_iter().enumerate() {
                let bytes = it.text.into_bytes();
                let len = bytes.len();
                let buf = new_c_buf(&bytes);
                *arr.add(i) = OcrLine {
                    text: buf,
                    text_len: len,
                    score: it.score,
                    top: it.frame.top,
                    left: it.frame.left,
                    width: it.frame.width,
                    height: it.frame.height,
                };
            }
            *lines = arr;
            *n = count;
            0
        },
        DetectTextResult::Spatial(_) => {
            set_err("意外的结果粒度".into());
            3
        }
    }
}

/// ocr_free_lines 释放 ocr_run 产出的数组
#[no_mangle]
pub extern "C" fn ocr_free_lines(p: *mut OcrLine, n: usize) {
    if p.is_null() {
        return;
    }
    unsafe {
        for i in 0..n {
            let line = &mut *p.add(i);
            if !line.text.is_null() {
                // text 是 Vec<u8> 的裸指针（len+1 容量），重建 Vec 让 Rust 回收
                drop(Vec::from_raw_parts(line.text, line.text_len + 1, line.text_len + 1));
            }
        }
        drop(Vec::from_raw_parts(p, n, n));
    }
}

/// ocr_free 释放 ocr_init 产出的句柄
#[no_mangle]
pub extern "C" fn ocr_free(h: *mut c_void) {
    if !h.is_null() {
        unsafe { drop(Box::from_raw(h as *mut Bridge)) }
    }
}

/// ocr_last_error 取本线程最近一次错误（UTF-8 写进 buf，不含 NUL）。
/// 返回写入长度；无错误返回 0。
#[no_mangle]
pub extern "C" fn ocr_last_error(buf: *mut u8, buf_len: usize) -> usize {
    let err = LAST_ERROR.with(|e| e.borrow_mut().take());
    let Some(err) = err else { return 0 };
    let bytes = err.as_bytes();
    let n = bytes.len().min(buf_len.saturating_sub(1));
    if !buf.is_null() && n > 0 {
        unsafe {
            std::ptr::copy_nonoverlapping(bytes.as_ptr(), buf, n);
            *buf.add(n) = 0;
        }
    }
    n
}

// ── 内部 ──

fn str_arg(p: *const u8, len: usize) -> Option<String> {
    if p.is_null() && len > 0 {
        set_err("空指针参数".into());
        return None;
    }
    match String::from_utf8(bytes_arg(p, len)?.to_vec()) {
        Ok(s) => Some(s),
        Err(_) => {
            set_err("路径不是 UTF-8".into());
            None
        }
    }
}

fn bytes_arg(p: *const u8, len: usize) -> Option<&'static [u8]> {
    if p.is_null() || len == 0 {
        set_err("空指针参数".into());
        return None;
    }
    Some(unsafe { std::slice::from_raw_parts(p, len) })
}

/// 分配 count 个 OcrLine 的数组。释放端用 Vec::from_raw_parts(p, n, n) 还原，
/// 所以这里的容量必须恰好是 count（with_capacity 保证，且 forget 前 len==cap==count
/// 的约定由 free 端按 n 重建，无需写满）。
fn libc_alloc_lines(count: usize) -> *mut OcrLine {
    let mut v: Vec<OcrLine> = Vec::with_capacity(count);
    let ptr: *mut OcrLine = v.as_mut_ptr();
    std::mem::forget(v);
    unsafe {
        for i in 0..count {
            std::ptr::write(ptr.add(i), OcrLine {
                text: std::ptr::null_mut(),
                text_len: 0,
                score: 0.0,
                top: 0.0,
                left: 0.0,
                width: 0.0,
                height: 0.0,
            });
        }
    }
    ptr
}

/// 分配 len+1 容量的缓冲（NUL 结尾）并拷入 bytes。
/// 释放端用 Vec::from_raw_parts(t, len+1, len+1) 还原，容量必须恰好 len+1。
fn new_c_buf(bytes: &[u8]) -> *mut u8 {
    let mut v = Vec::with_capacity(bytes.len() + 1);
    v.extend_from_slice(bytes);
    v.push(0);
    let ptr = v.as_mut_ptr();
    std::mem::forget(v);
    ptr
}
