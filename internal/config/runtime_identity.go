package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const runtimeIdentityFile = "runtime.json"

type RuntimeIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	Profile       string `json:"profile"`
}

func runtimeIdentityPath(stateDir string) string {
	return filepath.Join(stateDir, runtimeIdentityFile)
}

// ValidateRuntimeIdentity prevents a V2 profile from accidentally opening a
// V1 state root. Missing identity is treated as legacy V1 for compatibility.
func ValidateRuntimeIdentity(stateDir, profileName string) error {
	profileName = strings.ToLower(strings.TrimSpace(profileName))
	if profileName == "" {
		profileName = DefaultProfileName
	}
	data, err := os.ReadFile(runtimeIdentityPath(stateDir))
	if errors.Is(err, os.ErrNotExist) {
		if profileName == DefaultProfileName {
			return nil
		}
		return fmt.Errorf("state directory is not initialized for profile %s: %s", profileName, stateDir)
	}
	if err != nil {
		return err
	}
	var identity RuntimeIdentity
	if err := json.Unmarshal(data, &identity); err != nil {
		return fmt.Errorf("parse runtime identity: %w", err)
	}
	if identity.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported runtime identity schema_version %d", identity.SchemaVersion)
	}
	if identity.Profile == "" {
		return fmt.Errorf("runtime identity has no profile")
	}
	if identity.Profile != profileName {
		return fmt.Errorf("state directory belongs to profile %s, not %s", identity.Profile, profileName)
	}
	return nil
}

func EnsureRuntimeIdentity(stateDir, profileName string) error {
	profileName = strings.ToLower(strings.TrimSpace(profileName))
	if profileName == "" {
		profileName = DefaultProfileName
	}
	path := runtimeIdentityPath(stateDir)
	if _, err := os.Stat(path); err == nil {
		if err := ValidateRuntimeIdentity(stateDir, profileName); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(RuntimeIdentity{SchemaVersion: SchemaVersion, Profile: profileName})
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if existing, readErr := os.ReadFile(path); readErr == nil && string(existing) == string(data) {
		return nil
	}
	temporary, err := os.CreateTemp(stateDir, ".runtime-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}
