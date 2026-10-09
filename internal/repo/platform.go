package repo

import (
	"os"
	"runtime"
	"strings"
)

// Platform is the runtime the groups are evaluated against.
type Platform struct {
	GOOS string // runtime.GOOS value: "darwin", "linux", …
	WSL  bool   // Linux kernel running under Windows Subsystem for Linux
}

// CurrentPlatform detects the running platform.
func CurrentPlatform() Platform {
	p := Platform{GOOS: runtime.GOOS}
	if p.GOOS == "linux" {
		if b, err := os.ReadFile("/proc/version"); err == nil {
			p.WSL = strings.Contains(strings.ToLower(string(b)), "microsoft")
		}
	}
	return p
}

// validTargets mirrors Tuckr's VALID_TARGETS (src/dotfiles.rs), without the leading underscore.
var validTargets = map[string]bool{
	"wsl": true, "windows": true, "macos": true, "ios": true, "linux": true,
	"android": true, "freebsd": true, "dragonfly": true, "openbsd": true,
	"netbsd": true, "none": true, "unix": true,
}

// unixGOOS are the GOOS values Rust reports as target_family = "unix" among the targets we know.
var unixGOOS = map[string]bool{
	"darwin": true, "linux": true, "ios": true, "android": true,
	"freebsd": true, "dragonfly": true, "openbsd": true, "netbsd": true,
}

// SplitTarget returns the base name and the target suffix of a group directory name.
// A suffix that is not a Tuckr target makes the whole name a plain group.
func SplitTarget(name string) (base, target string) {
	i := strings.LastIndexByte(name, '_')
	if i <= 0 || i == len(name)-1 {
		return name, ""
	}
	t := name[i+1:]
	if !validTargets[t] {
		return name, ""
	}
	return name[:i], t
}

// Active reports whether a group with the given target suffix applies on p.
func (p Platform) Active(target string) bool {
	switch target {
	case "":
		return true
	case "unix":
		return unixGOOS[p.GOOS]
	case "macos":
		return p.GOOS == "darwin"
	case "wsl":
		return p.GOOS == "linux" && p.WSL
	case "windows", "none":
		return false
	default: // linux, ios, android, freebsd, … match GOOS by name
		return p.GOOS == target
	}
}

// priority mirrors Tuckr's get_group_priority: more specific targets win.
func priority(target string) int {
	switch target {
	case "":
		return 0
	case "unix", "windows":
		return 1
	case "wsl":
		return 3
	default:
		return 2
	}
}
