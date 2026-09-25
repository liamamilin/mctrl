package host

import (
	"os"

	"mctrl/internal/config"
)

type Info struct {
	Name        string `json:"name"`
	Hostname    string `json:"hostname"`
	Status      string `json:"status"`
	RemoteReady bool   `json:"remote_ready"`
}

func Current(cfg config.Config, remoteReady bool) Info {
	name, err := os.Hostname()
	if err != nil || name == "" {
		name = cfg.DeviceName
	}
	return Info{Name: cfg.DeviceName, Hostname: name, Status: "online", RemoteReady: remoteReady}
}
