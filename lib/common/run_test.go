package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetInstallPathWithConfPath(t *testing.T) {
	oldConf := ConfPath
	defer func() { ConfPath = oldConf }()

	ConfPath = filepath.Join(t.TempDir(), "custom-nps")
	if got := GetInstallPath(); got != ConfPath {
		t.Fatalf("GetInstallPath() = %q, want %q", got, ConfPath)
	}
}

func TestGetRunPathUsesInstallPathWhenExists(t *testing.T) {
	oldConf := ConfPath
	defer func() { ConfPath = oldConf }()
	oldArgs := append([]string(nil), os.Args...)
	defer func() { os.Args = oldArgs }()

	tmp := t.TempDir()
	ConfPath = tmp
	os.Args = []string{"sysuahb", "-c", "conf/sysuahb.conf"}

	if got := GetRunPath(); got != tmp {
		t.Fatalf("GetRunPath() = %q, want %q", got, tmp)
	}
}

func TestGetRunPathFallsBackToAppPathWhenInstallPathMissing(t *testing.T) {
	oldConf := ConfPath
	defer func() { ConfPath = oldConf }()
	oldArgs := append([]string(nil), os.Args...)
	defer func() { os.Args = oldArgs }()

	ConfPath = filepath.Join(t.TempDir(), "not-exist")
	os.Args = []string{"sysuahb", "-c", "conf/sysuahb.conf"}

	want := GetAppPath()
	if got := GetRunPath(); got != want {
		t.Fatalf("GetRunPath() = %q, want %q", got, want)
	}
}

func TestResolvePath(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join(".", "sysuahb.log"))
	if err != nil {
		t.Fatalf("filepath.Abs() error = %v", err)
	}
	if got := ResolvePath(abs); got != abs {
		t.Fatalf("ResolvePath() absolute = %q, want %q", got, abs)
	}

	oldConf := ConfPath
	defer func() { ConfPath = oldConf }()
	oldArgs := append([]string(nil), os.Args...)
	defer func() { os.Args = oldArgs }()

	runPath := t.TempDir()
	ConfPath = runPath
	os.Args = []string{"sysuahb", "-c", "conf/sysuahb.conf"}

	rel := "conf/sysuahb.conf"
	want := filepath.Join(runPath, rel)
	if got := ResolvePath(rel); got != want {
		t.Fatalf("ResolvePath() relative = %q, want %q", got, want)
	}
}

func TestRunTimeAndSecs(t *testing.T) {
	oldStart := StartTime
	defer func() { StartTime = oldStart }()

	StartTime = time.Now().Add(-(24*time.Hour + 2*time.Hour + 3*time.Minute + 4*time.Second))

	runtimeText := GetRunTime()
	for _, token := range []string{"1d", "2h", "3m", "4s"} {
		if !strings.Contains(runtimeText, token) {
			t.Fatalf("GetRunTime() = %q, missing %q", runtimeText, token)
		}
	}

	minInt := int64(24*3600 + 2*3600 + 3*60 + 4)
	if got := GetRunSecs(); got < minInt {
		t.Fatalf("GetRunSecs() = %d, want >= %d", got, minInt)
	}

	if got := GetStartTime(); got != StartTime.Unix() {
		t.Fatalf("GetStartTime() = %d, want %d", got, StartTime.Unix())
	}
}

func TestRunTimeZeroValueShowsSeconds(t *testing.T) {
	oldStart := StartTime
	defer func() { StartTime = oldStart }()

	StartTime = time.Now()
	if got := GetRunTime(); !strings.Contains(got, "s") {
		t.Fatalf("GetRunTime() = %q, want seconds suffix", got)
	}
}

func TestBinName(t *testing.T) {
	oldArgs := append([]string(nil), os.Args...)
	defer func() { os.Args = oldArgs }()

	cases := []struct{ arg0, want string }{
		{"sysuahb", "sysuahb"},
		{filepath.Join("usr", "bin", "sysuahb"), "sysuahb"},
		{filepath.Join("usr", "local", "bin", "sysficb.exe"), "sysficb"},
		{"sysabcd.EXE", "sysabcd"},
		{"npc", "npc"},
	}
	for _, c := range cases {
		os.Args = []string{c.arg0}
		if got := BinName(); got != c.want {
			t.Fatalf("BinName(%q) = %q, want %q", c.arg0, got, c.want)
		}
	}
}

func TestLogAndTmpPaths(t *testing.T) {
	oldArgs := append([]string(nil), os.Args...)
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"sysuahb", "-c", "conf/sysuahb.conf"}

	if IsWindows() {
		appPath := GetAppPath()
		if got := GetLogPath(); got != filepath.Join(appPath, "sysuahb.log") {
			t.Fatalf("GetLogPath() = %q, want %q", got, filepath.Join(appPath, "sysuahb.log"))
		}
		if got := GetClientLogPath(); got != filepath.Join(appPath, "sysuahb.log") {
			t.Fatalf("GetClientLogPath() = %q, want %q", got, filepath.Join(appPath, "sysuahb.log"))
		}
		if got := GetTmpPath(); got != appPath {
			t.Fatalf("GetTmpPath() = %q, want %q", got, appPath)
		}
		if got := GetConfigPath(); got != filepath.Join(appPath, "conf/sysuahb.conf") {
			t.Fatalf("GetConfigPath() = %q, want %q", got, filepath.Join(appPath, "conf/sysuahb.conf"))
		}
		return
	}

	if got := GetLogPath(); got != "/var/log/sysuahb.log" {
		t.Fatalf("GetLogPath() = %q, want %q", got, "/var/log/sysuahb.log")
	}
	if got := GetClientLogPath(); got != "/var/log/sysuahb.log" {
		t.Fatalf("GetClientLogPath() = %q, want %q", got, "/var/log/sysuahb.log")
	}
	if got := GetTmpPath(); got != "/tmp" {
		t.Fatalf("GetTmpPath() = %q, want %q", got, "/tmp")
	}
	if got := GetConfigPath(); got != "conf/sysuahb.conf" {
		t.Fatalf("GetConfigPath() = %q, want %q", got, "conf/sysuahb.conf")
	}
}
