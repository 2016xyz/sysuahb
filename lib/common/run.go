package common

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var ConfPath string
var StartTime = time.Now()

// GetRunPath Get the currently selected configuration file directory
// For non-Windows systems, select the /etc/<bin> as config directory if exist, or select ./
// windows system, select the C:\Program Files\<bin> as config directory if exist, or select ./
func GetRunPath() string {
	var path string
	if len(os.Args) == 1 {
		if !IsWindows() {
			dir, _ := filepath.Abs(filepath.Dir(os.Args[0])) //返回
			return dir + "/"
		} else {
			return "./"
		}
	} else {
		if path = GetInstallPath(); !FileExists(path) {
			return GetAppPath()
		}
	}
	return path
}

// BinName returns the current binary file name (without directory and extension).
// The service name, install path and log path all follow the binary name,
// so renaming the binary gives each installation a unique process name.
func BinName() string {
	name := filepath.Base(os.Args[0])
	if ext := filepath.Ext(name); strings.EqualFold(ext, ".exe") {
		name = strings.TrimSuffix(name, ext)
	}
	return name
}

// GetInstallPath Different systems get different installation paths
func GetInstallPath() string {
	var path string

	if ConfPath != "" {
		return ConfPath
	}

	if IsWindows() {
		path = `C:\Program Files\` + BinName()
	} else {
		path = "/etc/" + BinName()
	}

	return path
}

// GetAppPath Get the absolute path to the running directory
func GetAppPath() string {
	if exePath, err := os.Executable(); err == nil {
		return filepath.Dir(exePath)
	}
	if path, err := filepath.Abs(filepath.Dir(os.Args[0])); err == nil {
		return path
	}
	return os.Args[0]
}

// IsWindows Determine whether the current system is a Windows system?
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// GetLogPath interface log file path
func GetLogPath() string {
	var path string
	if IsWindows() {
		path = filepath.Join(GetAppPath(), BinName()+".log")
	} else {
		path = "/var/log/" + BinName() + ".log"
	}
	return path
}

// GetNpcLogPath interface npc log file path
func GetNpcLogPath() string {
	var path string
	if IsWindows() {
		path = filepath.Join(GetAppPath(), BinName()+".log")
	} else {
		path = "/var/log/" + BinName() + ".log"
	}
	return path
}

// GetTmpPath interface pid file path
func GetTmpPath() string {
	var path string
	if IsWindows() {
		path = GetAppPath()
	} else {
		path = "/tmp"
	}
	return path
}

// GetConfigPath config file path
func GetConfigPath() string {
	var path string
	if IsWindows() {
		path = filepath.Join(GetAppPath(), "conf/sysficb.conf")
	} else {
		path = "conf/sysficb.conf"
	}
	return path
}

func ResolvePath(path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(GetRunPath(), path)
	}
	return path
}

func GetRunTime() string {
	totalSecs := int64(time.Since(StartTime).Seconds())
	days := totalSecs / 86400
	totalSecs %= 86400
	hours := totalSecs / 3600
	totalSecs %= 3600
	mins := totalSecs / 60
	secs := totalSecs % 60
	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if secs > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}
	return strings.Join(parts, " ")
}

func GetRunSecs() int64 {
	return int64(time.Since(StartTime).Seconds())
}

func GetStartTime() int64 {
	return StartTime.Unix()
}
