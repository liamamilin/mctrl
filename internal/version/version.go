package version

import (
	"fmt"
	"runtime"
)

var (
	Version   = "dev"
	Commit    = "none"
	BuildTime = "unknown"
)

func String() string {
	return fmt.Sprintf("%s (commit=%s, built=%s, %s/%s)", Version, Commit, BuildTime, runtime.GOOS, runtime.GOARCH)
}
