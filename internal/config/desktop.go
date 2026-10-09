package config

import (
	"fmt"
	"mctrl/internal/storage"
	"path/filepath"
)

// A local connection hint, not Session or Work authority. The daemon records
// its effective socket so a desktop shell need not inherit launchd's env.
type DesktopConnection struct {
	SchemaVersion int    `json:"schema_version"`
	Profile       string `json:"profile"`
	Socket        string `json:"socket"`
}

func SaveDesktopConnection(root, profile, socket string) error {
	return storage.WriteJSONAtomic(filepath.Join(root, "desktop.json"), DesktopConnection{SchemaVersion: SchemaVersion, Profile: profile, Socket: socket}, 0600)
}

func LoadDesktopConnection(root, profile string) (DesktopConnection, error) {
	var value DesktopConnection
	if err := storage.ReadJSON(filepath.Join(root, "desktop.json"), &value); err != nil {
		return value, err
	}
	if value.SchemaVersion != SchemaVersion || value.Profile != profile {
		return value, fmt.Errorf("desktop connection does not match profile %s; restart its daemon", profile)
	}
	return value, nil
}
