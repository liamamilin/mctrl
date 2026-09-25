package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	// ProfileEnv selects an isolated mctrl instance without changing the
	// existing MCTRL_HOME override used by tests and recovery tooling.
	ProfileEnv         = "MCTRL_PROFILE"
	DefaultProfileName = "v1"
)

// Profile describes the runtime identity of one mctrl installation. The V1
// values intentionally retain the historical state path, port and launchd
// label so adding profiles does not move an existing installation.
type Profile struct {
	Name              string
	DefaultPort       int
	LaunchAgentLabel  string
	DefaultTmuxSocket string
	PairAppName       string
	BundleIdentifier  string
}

func ProfileForName(name string) (Profile, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		name = DefaultProfileName
	}
	if !strings.HasPrefix(name, "v") {
		return Profile{}, fmt.Errorf("profile must be v1, v2, v3, ...: %q", name)
	}
	index, err := strconv.Atoi(strings.TrimPrefix(name, "v"))
	if err != nil || index < 1 || index > 9 || "v"+strconv.Itoa(index) != name {
		return Profile{}, fmt.Errorf("profile must be v1, v2, v3, ... v9: %q", name)
	}
	if name == DefaultProfileName {
		return Profile{
			Name:              name,
			DefaultPort:       7681,
			LaunchAgentLabel:  "com.mctrl.daemon",
			DefaultTmuxSocket: "",
			PairAppName:       "Mctrl Pair",
			BundleIdentifier:  "com.mctrl.pair-launcher",
		}, nil
	}
	return Profile{
		Name:              name,
		DefaultPort:       17680 + index,
		LaunchAgentLabel:  "com.mctrl." + name + ".daemon",
		DefaultTmuxSocket: fmt.Sprintf("/private/tmp/tmux-%d/mctrl-%s", os.Getuid(), name),
		PairAppName:       "Mctrl " + strings.ToUpper(name) + " Pair",
		BundleIdentifier:  "com.mctrl." + name + ".pair-launcher",
	}, nil
}

func ActiveProfile() (Profile, error) {
	name := strings.TrimSpace(os.Getenv(ProfileEnv))
	if name == "" {
		name = DefaultProfileName
	}
	return ProfileForName(name)
}

func (p Profile) StateDir(home string) string {
	if p.Name == DefaultProfileName {
		return filepath.Join(home, ".mctrl")
	}
	return filepath.Join(home, ".mctrl-"+p.Name)
}

func (p Profile) ApplyEnvironment() error {
	if p.DefaultTmuxSocket == "" || strings.TrimSpace(os.Getenv("MCTRL_TMUX_SOCKET")) != "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(p.DefaultTmuxSocket), 0700); err != nil {
		return fmt.Errorf("create profile tmux socket directory: %w", err)
	}
	if err := os.Setenv("MCTRL_TMUX_SOCKET", p.DefaultTmuxSocket); err != nil {
		return fmt.Errorf("set profile tmux socket: %w", err)
	}
	return nil
}

func DefaultForProfile(name string) (Config, error) {
	profile, err := ProfileForName(name)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	cfg.Port = profile.DefaultPort
	return cfg, nil
}
