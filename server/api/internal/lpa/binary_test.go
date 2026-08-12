package lpa

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveBinaryFindsBundledTool(t *testing.T) {
	if stringsTrimmed := os.Getenv("LPAC_PATH"); stringsTrimmed != "" {
		t.Setenv("LPAC_PATH", "")
	}
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "server", "api")
	if err := os.MkdirAll(workingDirectory, 0755); err != nil {
		t.Fatal(err)
	}
	binaryName := "lpac"
	if runtime.GOOS == "windows" {
		binaryName = "lpac.exe"
	}
	binary := filepath.Join(root, "server", "tools", "lpac", binaryName)
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workingDirectory); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDirectory) })

	resolved := resolveBinary()
	expected, _ := filepath.Abs(binary)
	if resolved != expected {
		t.Fatalf("resolveBinary() = %q, want %q", resolved, expected)
	}
}

func TestResolveBinaryPrefersEnvironment(t *testing.T) {
	configured := filepath.Join(t.TempDir(), "custom-lpac")
	t.Setenv("LPAC_PATH", configured)
	resolved := resolveBinary()
	expected, _ := filepath.Abs(configured)
	if resolved != expected {
		t.Fatalf("resolveBinary() = %q, want %q", resolved, expected)
	}
}
