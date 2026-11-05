package cli

import "runtime/debug"

func versionString() string {
	if Version != "dev" {
		return Version
	}
	// go install module@version supplies this without release-specific ldflags.
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}
