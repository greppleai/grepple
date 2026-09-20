package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// Version is the release tag or source description injected at build time.
var Version = "dev"

// Commit is the source revision injected at build time.
var Commit = "unknown"

// BuildDate is the source commit timestamp injected at build time. Using the
// commit timestamp instead of the wall-clock build time keeps builds reproducible.
var BuildDate = "unknown"

// String returns reproducible version and runtime metadata.
func String() string {
	version, commit, buildDate := Version, Commit, BuildDate
	if info, ok := debug.ReadBuildInfo(); ok {
		version = preferModuleVersion(version, info.Main.Version)
		version, commit, buildDate = applyBuildSettings(version, commit, buildDate, info.Settings)
	}
	return fmt.Sprintf("grepple %s (commit %s, source-date %s, %s, %s/%s)", version, commit, buildDate, runtime.Version(), runtime.GOOS, runtime.GOARCH)
}

func preferModuleVersion(version, moduleVersion string) string {
	if version == "dev" && moduleVersion != "" && moduleVersion != "(devel)" {
		return moduleVersion
	}
	return version
}

func applyBuildSettings(version, commit, buildDate string, settings []debug.BuildSetting) (string, string, string) {
	for _, setting := range settings {
		switch setting.Key {
		case "vcs.revision":
			commit = preferKnownValue(commit, setting.Value)
		case "vcs.time":
			buildDate = preferKnownValue(buildDate, setting.Value)
		case "vcs.modified":
			version = markDirtyVersion(version, setting.Value)
		}
	}
	return version, commit, buildDate
}

func preferKnownValue(current, discovered string) string {
	if current == "unknown" {
		return discovered
	}
	return current
}

func markDirtyVersion(version, modified string) string {
	if modified == "true" && !strings.HasSuffix(version, "-dirty") {
		return version + "-dirty"
	}
	return version
}
