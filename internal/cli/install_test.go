package cli

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBinFileName(t *testing.T) {
	name := binFileName()
	if runtime.GOOS == "windows" && name != "cloudns.exe" {
		t.Fatalf("name = %q", name)
	}
	if runtime.GOOS != "windows" && name != "cloudns" {
		t.Fatalf("name = %q", name)
	}
}

func TestBinDirs(t *testing.T) {
	dirs := binDirs()
	if len(dirs) == 0 {
		t.Fatal("no bin directories")
	}
	if runtime.GOOS == "windows" {
		if !strings.HasSuffix(dirs[0], string(filepath.Separator)+"bin") && filepath.Base(dirs[0]) != "bin" {
			t.Fatalf("windows dir = %q", dirs[0])
		}
		return
	}
	if dirs[0] != "/usr/local/bin" || dirs[1] != "/usr/bin" {
		t.Fatalf("dirs = %#v", dirs)
	}
}

func TestDirOnPath(t *testing.T) {
	t.Setenv("PATH", "/usr/bin"+string(filepath.ListSeparator)+"/tmp/cloudns-bin")
	if !dirOnPath("/tmp/cloudns-bin") {
		t.Fatal("expected /tmp/cloudns-bin on PATH")
	}
	if dirOnPath("/nowhere") {
		t.Fatal("expected /nowhere off PATH")
	}
}
