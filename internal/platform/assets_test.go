package platform

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func useModelRoot(t *testing.T, ver string) string {
	t.Helper()
	root := t.TempDir()
	oldRoot, oldVer := tempRoot, version
	tempRoot = func() string { return root }
	version = ver
	t.Cleanup(func() { tempRoot, version = oldRoot, oldVer })
	return root
}

func TestWriteAssetsReusesVersionDir(t *testing.T) {
	root := useModelRoot(t, "1.2.3")
	if err := writeAssets(); err != nil {
		t.Fatal(err)
	}
	det := filepath.Join(root, "models", "1.2.3", "det.onnx")
	if st, err := os.Stat(det); err != nil || st.Size() != int64(len(detOnnx)) {
		t.Fatalf("det.onnx 没有正确写出: %v", err)
	}

	// 完整的一份不该重写：用固定的旧时间戳验证文件没有被碰过。
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(det, past, past); err != nil {
		t.Fatal(err)
	}
	if err := writeAssets(); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(det); !st.ModTime().Equal(past) {
		t.Error("已有完整模型却又重写了一遍")
	}

	// 缺文件（比如上次写到一半）就补齐。
	if err := os.Remove(det); err != nil {
		t.Fatal(err)
	}
	if err := writeAssets(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(det); err != nil {
		t.Errorf("缺文件时没有补齐: %v", err)
	}
}

func TestWriteAssetsSeparatesVersions(t *testing.T) {
	root := useModelRoot(t, "1.0.0")
	if err := writeAssets(); err != nil {
		t.Fatal(err)
	}
	version = "2.0.0"
	if err := writeAssets(); err != nil {
		t.Fatal(err)
	}
	for _, ver := range []string{"1.0.0", "2.0.0"} {
		if _, err := os.Stat(filepath.Join(root, "models", ver, "dict.txt")); err != nil {
			t.Fatalf("版本 %s 的模型目录不完整: %v", ver, err)
		}
	}

	pruneOldModels()
	if _, err := os.Stat(filepath.Join(root, "models", "1.0.0")); !os.IsNotExist(err) {
		t.Error("旧版本模型目录没有被清掉")
	}
	if _, err := os.Stat(filepath.Join(root, "models", "2.0.0", "dict.txt")); err != nil {
		t.Errorf("当前版本模型被误删: %v", err)
	}
}
