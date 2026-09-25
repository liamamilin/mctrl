package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestStringIncludesBuildIdentity(t *testing.T) {
	oldVersion, oldCommit, oldBuild := Version, Commit, BuildTime
	defer func() {
		Version, Commit, BuildTime = oldVersion, oldCommit, oldBuild
	}()
	Version, Commit, BuildTime = "1.2.3", "abc123", "2026-09-25T00:00:00Z"
	got := String()
	for _, expected := range []string{"1.2.3", "abc123", "2026-09-25T00:00:00Z", runtime.GOOS, runtime.GOARCH} {
		if !strings.Contains(got, expected) {
			t.Fatalf("version string %q does not contain %q", got, expected)
		}
	}
}
