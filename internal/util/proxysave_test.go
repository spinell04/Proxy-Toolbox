package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	lines := []string{"1.2.3.4:8080:u:p", "5.6.7.8:9090:u:p"}

	if err := WriteLines(path, lines); err != nil {
		t.Fatalf("WriteLines: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "1.2.3.4:8080:u:p\n5.6.7.8:9090:u:p\n"
	if string(got) != want {
		t.Errorf("file = %q, want %q", string(got), want)
	}
}
