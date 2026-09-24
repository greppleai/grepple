package archdaemon

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// sourceIdentity rejects a worker built from a different source revision or
// Go toolchain. Protocol changes must also bump the explicit protocol schema.
func sourceIdentity() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return protocol + ":" + runtime.Version()
	}
	revision, modified := "unknown", "unknown"
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value
		}
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s", protocol, info.Main.Version, revision, modified, runtime.Version())
}
