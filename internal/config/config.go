package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const SchemaVersion = 1

type TransportProfile string

const (
	TransportTrustedLANHTTP TransportProfile = "trusted_lan_http"
	TransportTLSTerminated  TransportProfile = "tls_terminated"
)

type RemoteAvailability string

const (
	AvailabilityOnAC     RemoteAvailability = "on_ac"
	AvailabilityWorkOnly RemoteAvailability = "work_only"
	AvailabilityAlways   RemoteAvailability = "always"
)

// TerminalSize is a terminal geometry in character cells. It describes the
// canonical full Session size, never the phone's own viewport.
type TerminalSize struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

const (
	// These bounds apply to every terminal geometry, configured or requested.
	MinTerminalCols = 20
	MaxTerminalCols = 500
	MinTerminalRows = 10
	MaxTerminalRows = 200

	// DefaultFullTerminalSize is deliberately close to a real desktop terminal.
	// Programs lay themselves out by width and drop panels below their own
	// thresholds, so a "full" phone view that is too narrow shows strictly less
	// than the desktop. A Session whose pane is already larger still wins over
	// this value; see internal/terminal.FullWinsize.
	DefaultFullTerminalCols = 240
	DefaultFullTerminalRows = 60
)

// DefaultFullTerminalSize is the canonical full Session size used when a config
// file does not specify one.
func DefaultFullTerminalSize() TerminalSize {
	return TerminalSize{Cols: DefaultFullTerminalCols, Rows: DefaultFullTerminalRows}
}

// Validate reports whether the size can be laid out by a terminal.
func (s TerminalSize) Validate() error {
	if s.Cols < MinTerminalCols || s.Cols > MaxTerminalCols {
		return fmt.Errorf("terminal cols must be between %d and %d, got %d", MinTerminalCols, MaxTerminalCols, s.Cols)
	}
	if s.Rows < MinTerminalRows || s.Rows > MaxTerminalRows {
		return fmt.Errorf("terminal rows must be between %d and %d, got %d", MinTerminalRows, MaxTerminalRows, s.Rows)
	}
	return nil
}

// Clamped returns the size limited to the supported range. Callers that accept
// untrusted input should prefer Validate and surface the error instead.
func (s TerminalSize) Clamped() TerminalSize {
	clamp := func(value, low, high int) int {
		if value < low {
			return low
		}
		if value > high {
			return high
		}
		return value
	}
	return TerminalSize{
		Cols: clamp(s.Cols, MinTerminalCols, MaxTerminalCols),
		Rows: clamp(s.Rows, MinTerminalRows, MaxTerminalRows),
	}
}

type Config struct {
	SchemaVersion      int                `json:"schema_version"`
	DeviceName         string             `json:"device_name"`
	ListenAddress      string             `json:"listen_address"`
	Port               int                `json:"port"`
	TransportProfile   TransportProfile   `json:"transport_profile"`
	RemoteAvailability RemoteAvailability `json:"remote_availability"`
	StartAfterLogin    bool               `json:"start_after_login"`
	StorePrompts       bool               `json:"store_prompts"`
	TerminalFullSize   TerminalSize       `json:"terminal_full_size"`
	PublicURL          string             `json:"public_url,omitempty"`
	AllowedOrigins     []string           `json:"allowed_origins,omitempty"`
}

// Default deliberately binds to loopback. LAN access is an explicit setup choice.
func Default() Config {
	return Config{
		SchemaVersion:      SchemaVersion,
		DeviceName:         defaultDeviceName(),
		ListenAddress:      "127.0.0.1",
		Port:               7681,
		TransportProfile:   TransportTrustedLANHTTP,
		RemoteAvailability: AvailabilityOnAC,
		StartAfterLogin:    true,
		StorePrompts:       false,
		TerminalFullSize:   DefaultFullTerminalSize(),
		PublicURL:          "",
		AllowedOrigins:     nil,
	}
}

func defaultDeviceName() string {
	if name := strings.TrimSpace(os.Getenv("MCTRL_DEVICE_NAME")); name != "" {
		return name
	}
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "Mac"
	}
	return host
}

func (c Config) Validate() error {
	if c.SchemaVersion == 0 {
		return errors.New("schema_version is required")
	}
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported schema_version %d", c.SchemaVersion)
	}
	if strings.TrimSpace(c.DeviceName) == "" {
		return errors.New("device_name is required")
	}
	if strings.TrimSpace(c.ListenAddress) == "" {
		return errors.New("listen_address is required")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}
	if err := c.TerminalFullSize.Validate(); err != nil {
		return fmt.Errorf("terminal_full_size: %w", err)
	}
	switch c.TransportProfile {
	case TransportTrustedLANHTTP, TransportTLSTerminated:
	default:
		return fmt.Errorf("unsupported transport_profile %q", c.TransportProfile)
	}
	switch c.RemoteAvailability {
	case AvailabilityOnAC, AvailabilityWorkOnly, AvailabilityAlways:
	default:
		return fmt.Errorf("unsupported remote_availability %q", c.RemoteAvailability)
	}
	if c.TransportProfile == TransportTLSTerminated && !isLoopbackAddress(c.ListenAddress) {
		return fmt.Errorf("tls_terminated requires a loopback listener; configure TLS termination on the same Mac")
	}
	if c.TransportProfile == TransportTLSTerminated {
		if strings.TrimSpace(c.PublicURL) == "" {
			return errors.New("tls_terminated requires public_url for pairing and origin validation")
		}
		if err := validateOriginURL(c.PublicURL, "public_url"); err != nil {
			return err
		}
		if parsed, _ := url.Parse(strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")); parsed.Scheme != "https" {
			return errors.New("public_url must use https for tls_terminated")
		}
	} else if strings.TrimSpace(c.PublicURL) != "" {
		if err := validateOriginURL(c.PublicURL, "public_url"); err != nil {
			return err
		}
		if parsed, _ := url.Parse(strings.TrimRight(strings.TrimSpace(c.PublicURL), "/")); parsed.Scheme != "http" {
			return errors.New("public_url must use http for trusted_lan_http; select tls_terminated for HTTPS")
		}
	}
	for _, origin := range c.AllowedOrigins {
		if err := validateOriginURL(origin, "allowed origin"); err != nil {
			return err
		}
		parsed, _ := url.Parse(strings.TrimRight(strings.TrimSpace(origin), "/"))
		if c.TransportProfile == TransportTLSTerminated && parsed.Scheme != "https" {
			return fmt.Errorf("allowed origin must use https for tls_terminated: %q", origin)
		}
		if c.TransportProfile == TransportTrustedLANHTTP && parsed.Scheme != "http" {
			return fmt.Errorf("allowed origin must use http for trusted_lan_http: %q", origin)
		}
	}
	return nil
}

func validateOriginURL(value, label string) error {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an http(s) origin: %q", label, value)
	}
	return nil
}

func isLoopbackAddress(address string) bool {
	address = strings.TrimSpace(address)
	if strings.EqualFold(address, "localhost") {
		return true
	}
	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}

func (c Config) Address() string {
	return net.JoinHostPort(c.ListenAddress, strconv.Itoa(c.Port))
}

func AdvertisedPairURL(c Config) string {
	if publicURL := strings.TrimRight(strings.TrimSpace(c.PublicURL), "/"); publicURL != "" {
		return publicURL + "/pair"
	}
	scheme := "http"
	if c.TransportProfile == TransportTLSTerminated {
		scheme = "https"
	}
	host := c.ListenAddress
	if host == "" || host == "0.0.0.0" || host == "::" {
		if address := lanAddress(); address != "" {
			host = address
		} else if name, err := os.Hostname(); err == nil && name != "" {
			host = name
		} else {
			host = "127.0.0.1"
		}
	}
	return scheme + "://" + net.JoinHostPort(host, strconv.Itoa(c.Port)) + "/pair"
}

func lanAddress() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			ipNet, ok := address.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.To4() == nil {
				continue
			}
			return ipNet.IP.String()
		}
	}
	return ""
}

func StateDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv("MCTRL_HOME")); value != "" {
		absolute, err := filepath.Abs(value)
		if err != nil {
			return "", fmt.Errorf("resolve MCTRL_HOME: %w", err)
		}
		return absolute, nil
	}
	profile, err := ActiveProfile()
	if err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return profile.StateDir(home), nil
}

func EnsureStateDir() (string, error) {
	dir, err := StateDir()
	if err != nil {
		return "", err
	}
	for _, subdir := range []string{"", "work", "logs", "logs/runner"} {
		if err := os.MkdirAll(filepath.Join(dir, subdir), 0700); err != nil {
			return "", fmt.Errorf("create state directory: %w", err)
		}
	}
	profile, err := ActiveProfile()
	if err != nil {
		return "", err
	}
	if profile.Name != DefaultProfileName {
		_, markerErr := os.Stat(filepath.Join(dir, runtimeIdentityFile))
		_, configErr := os.Stat(filepath.Join(dir, "config.json"))
		if errors.Is(markerErr, os.ErrNotExist) && configErr == nil {
			return "", fmt.Errorf("refusing unmarked profile state directory: %s", dir)
		}
		if markerErr != nil && !errors.Is(markerErr, os.ErrNotExist) {
			return "", markerErr
		}
	}
	if err := EnsureRuntimeIdentity(dir, profile.Name); err != nil {
		return "", err
	}
	return dir, nil
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// withDefaults fills in fields a config file written before they existed.
// Without it an older config would fail validation on a zero value and the
// daemon would refuse to start.
func (c Config) withDefaults() Config {
	if c.TerminalFullSize == (TerminalSize{}) {
		c.TerminalFullSize = DefaultFullTerminalSize()
	}
	return c
}

func Save(path string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return writeAtomic(path, data, 0600)
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mctrl-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
