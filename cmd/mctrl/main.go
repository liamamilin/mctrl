package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mctrl/internal/api"
	"mctrl/internal/auth"
	"mctrl/internal/config"
	"mctrl/internal/power"
	"mctrl/internal/project"
	"mctrl/internal/runner"
	"mctrl/internal/tmux"
	"mctrl/internal/version"
	"mctrl/internal/work"

	qrcode "github.com/skip2/go-qrcode"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	command := os.Args[1]
	args := os.Args[2:]
	var err error
	switch command {
	case "serve":
		err = runServe(args)
	case "setup":
		err = runSetup(args)
	case "status":
		err = runStatus(args)
	case "pair":
		err = runPair(args)
	case "doctor":
		err = runDoctor(args)
	case "logs":
		err = runLogs(args)
	case "project":
		err = runProject(args)
	case "devices":
		err = runDevices(args)
	case "revoke":
		err = runRevoke(args)
	case "restart":
		err = runRestart(args)
	case "uninstall":
		err = runUninstall(args)
	case "help", "-h", "--help":
		usage()
		return
	case "version", "--version":
		fmt.Println(version.String())
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mctrl:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`mctrl - mobile control surface for terminal work

Commands:
  mctrl version     print build version
  mctrl setup       initialize state and start the daemon
  mctrl status      show daemon, tmux, projects, and device status
  mctrl pair        create a short-lived pairing token
  mctrl doctor      diagnose the local installation
  mctrl logs        show daemon logs
  mctrl project     add/remove/list registered Projects
  mctrl devices     list paired devices
  mctrl revoke      revoke a paired device
  mctrl restart     restart only the daemon
  mctrl uninstall   stop mctrl without destroying arbitrary tmux sessions

Internal:
  mctrl serve       run the daemon in the foreground`)
}

func runServe(_ []string) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(stateDir)
	if err != nil {
		return err
	}
	lock, err := acquireDaemonLock(stateDir)
	if err != nil {
		return err
	}
	defer lock.release()
	server, err := api.NewServer(cfg, stateDir)
	if err != nil {
		return err
	}
	defer server.Close()
	store := work.NewStore(stateDir)
	adapter := tmux.New()
	reconcile := func() error {
		reconcileContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return work.Reconcile(reconcileContext, store, adapter)
	}
	if err := reconcile(); err != nil {
		server.RecordReconcileError(err)
		fmt.Fprintln(os.Stderr, "warning: initial reconciliation:", err)
	}
	pidPath := filepath.Join(stateDir, "daemon.pid")
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0600)
	defer os.Remove(pidPath)

	listener, err := newListener(cfg.Address())
	if err != nil {
		return err
	}
	defer listener.Close()
	fmt.Printf("mctrl listening on http://%s\n", cfg.Address())

	reconcileDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if reconcileErr := reconcile(); reconcileErr != nil {
					server.RecordReconcileError(reconcileErr)
					fmt.Fprintln(os.Stderr, "warning: periodic reconciliation:", reconcileErr)
				} else {
					server.RecordReconcileError(nil)
				}
			case <-reconcileDone:
				return
			}
		}
	}()
	defer close(reconcileDone)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	httpServer := &http.Server{Handler: server, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 1 << 20}
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.Serve(listener) }()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stop:
		// Finish in-flight control requests without touching tmux, runners,
		// or any other durable process.
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownContext)
		return nil
	}
}

func runSetup(args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("mctrl V1 setup supports macOS; current platform is %s", runtime.GOOS)
	}
	fs := flagSet("setup")
	lan := fs.Bool("lan", false, "listen on all interfaces (Trusted-LAN HTTP)")
	address := fs.String("address", "", "override listen address")
	port := fs.Int("port", 0, "override port")
	transport := fs.String("transport", "", "transport profile: trusted_lan_http or tls_terminated")
	publicURL := fs.String("public-url", "", "browser-facing origin, required for tls_terminated")
	remoteAvailability := fs.String("remote-availability", "", "remote availability: on_ac, work_only, or always")
	noStart := fs.Bool("no-start", false, "initialize without starting daemon")
	startAfterLogin := fs.Bool("start-after-login", false, "install a login LaunchAgent")
	noStartAfterLogin := fs.Bool("no-start-after-login", false, "do not install a login LaunchAgent")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *startAfterLogin && *noStartAfterLogin {
		return errors.New("--start-after-login and --no-start-after-login are mutually exclusive")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		return fmt.Errorf("tmux is required: %w", err)
	}
	stateDir, err := config.EnsureStateDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(stateDir, "config.json")
	cfg, err := config.Load(cfgPath)
	if errors.Is(err, os.ErrNotExist) {
		cfg = config.Default()
	} else if err != nil {
		return err
	}
	if *address != "" {
		cfg.ListenAddress = *address
	} else if *lan {
		cfg.ListenAddress = "0.0.0.0"
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *transport != "" {
		cfg.TransportProfile = config.TransportProfile(*transport)
	}
	if *publicURL != "" {
		cfg.PublicURL = strings.TrimSpace(*publicURL)
	}
	if cfg.TransportProfile == config.TransportTrustedLANHTTP && strings.HasPrefix(strings.ToLower(cfg.PublicURL), "https://") {
		cfg.PublicURL = ""
	}
	if *startAfterLogin {
		cfg.StartAfterLogin = true
	}
	if *noStartAfterLogin {
		cfg.StartAfterLogin = false
	}
	if cfg.TransportProfile == config.TransportTLSTerminated && strings.TrimSpace(cfg.PublicURL) == "" {
		return errors.New("tls_terminated requires --public-url, for example https://mctrl.example.internal")
	}
	if *remoteAvailability != "" {
		cfg.RemoteAvailability = config.RemoteAvailability(*remoteAvailability)
	}
	if err := config.Save(cfgPath, cfg); err != nil {
		return err
	}
	if executable, executableErr := os.Executable(); executableErr == nil {
		runnerPath := filepath.Join(filepath.Dir(executable), "mctrl-runner")
		if _, runnerErr := os.Stat(runnerPath); runnerErr != nil {
			fmt.Fprintf(os.Stderr, "warning: mctrl-runner is not next to mctrl (%s); build/install both binaries before launching Work\n", runnerPath)
		}
	}
	fmt.Printf("State directory: %s\n", stateDir)
	fmt.Printf("Address: %s\n", strings.TrimSuffix(advertisedPairURL(cfg), "/pair"))
	if cfg.TransportProfile == config.TransportTrustedLANHTTP {
		fmt.Println("Transport: Trusted-LAN HTTP (no end-to-end encryption; use only on a trusted network)")
	} else {
		fmt.Println("Transport: TLS-terminated HTTPS/WSS through the configured public URL")
	}
	if !*noStart {
		stopDaemon(stateDir)
		if cfg.StartAfterLogin {
			if err := installLaunchAgent(); err != nil {
				fmt.Fprintf(os.Stderr, "warning: LaunchAgent could not be loaded: %v; starting a background process instead\n", err)
				if startErr := startBackground(); startErr != nil {
					return startErr
				}
				fmt.Println("Daemon: running in background fallback")
			} else {
				fmt.Println("Daemon: started through launchd")
			}
		} else {
			if err := removeLaunchAgent(); err != nil {
				return fmt.Errorf("disable login LaunchAgent: %w", err)
			}
			if err := startBackground(); err != nil {
				return err
			}
			fmt.Println("Daemon: started in background (login LaunchAgent disabled)")
		}
	}
	fmt.Println("Next: run `mctrl pair` and open the printed pairing URL on the phone.")
	return nil
}

type daemonLock struct {
	path  string
	token string
}

func acquireDaemonLock(stateDir string) (*daemonLock, error) {
	path := filepath.Join(stateDir, "daemon.lock")
	for attempt := 0; attempt < 50; attempt++ {
		err := os.Mkdir(path, 0700)
		if err == nil {
			tokenBytes := make([]byte, 16)
			if _, randomErr := rand.Read(tokenBytes); randomErr != nil {
				_ = os.Remove(path)
				return nil, randomErr
			}
			token := hex.EncodeToString(tokenBytes)
			_ = os.WriteFile(filepath.Join(path, "token"), []byte(token), 0600)
			_ = os.WriteFile(filepath.Join(path, "pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0600)
			return &daemonLock{path: path, token: token}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		pid := readPIDFromFile(filepath.Join(path, "pid"))
		if pid > 0 && isMctrlProcess(pid) {
			return nil, fmt.Errorf("mctrl daemon is already running (pid %d)", pid)
		}
		_ = os.RemoveAll(path)
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("timed out acquiring daemon lock")
}

func (l *daemonLock) release() {
	if l == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(l.path, "token"))
	if err != nil || strings.TrimSpace(string(data)) != l.token {
		return
	}
	_ = os.RemoveAll(l.path)
}

func readPIDFromFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func isMctrlProcess(pid int) bool {
	if pid <= 0 || !processAlive(pid) {
		return false
	}
	output, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return false
	}
	command := strings.ToLower(string(output))
	return strings.Contains(command, "mctrl") && strings.Contains(command, "serve")
}

func stopDaemon(stateDir string) {
	pid := readPID(stateDir)
	if pid <= 0 || !isMctrlProcess(pid) {
		return
	}
	_ = syscall.Kill(pid, syscall.SIGTERM)
	for i := 0; i < 30 && processAlive(pid); i++ {
		time.Sleep(100 * time.Millisecond)
	}
}

func runStatus(_ []string) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(stateDir)
	if err != nil {
		return err
	}
	address := localAddress(cfg)
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + address + "/healthz")
	serverStatus := "stopped"
	if err == nil {
		var health struct {
			Status string `json:"status"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&health)
		_ = response.Body.Close()
		if response.StatusCode == http.StatusOK {
			serverStatus = "running"
			if health.Status == "degraded" {
				serverStatus = "degraded"
			}
		}
	}
	sessions, sessionErr := tmux.New().ListSessions(context.Background())
	projects, projectErr := project.NewStore(stateDir).List()
	devices, deviceErr := auth.NewRegistry(stateDir).List()
	fmt.Printf("Server       %s\n", serverStatus)
	fmt.Printf("Address      http://%s\n", cfg.Address())
	fmt.Printf("Transport    %s\n", transportLabel(cfg.TransportProfile))
	if cfg.PublicURL != "" {
		fmt.Printf("Public URL   %s\n", cfg.PublicURL)
	}
	fmt.Printf("At login     %t\n", cfg.StartAfterLogin)
	remoteReady := "off"
	if serverStatus == "running" {
		switch cfg.RemoteAvailability {
		case config.AvailabilityAlways:
			remoteReady = "always"
		case config.AvailabilityOnAC:
			if power.OnAC() {
				remoteReady = "on_ac"
			}
		case config.AvailabilityWorkOnly:
			if items, listErr := work.NewStore(stateDir).List(); listErr == nil {
				for _, item := range items {
					if item.KeepAwake && (item.State == work.StateStarting || item.State == work.StateRunning) && power.WorkAssertionActive(item.RunnerPID) {
						remoteReady = "work"
						break
					}
				}
			}
		}
	}
	fmt.Printf("RemoteReady  %s\n", remoteReady)
	if sessionErr != nil {
		fmt.Printf("Sessions     unavailable (%v)\n", sessionErr)
	} else {
		fmt.Printf("Sessions     %d\n", len(sessions))
	}
	if projectErr != nil {
		fmt.Printf("Projects     unavailable (%v)\n", projectErr)
	} else {
		fmt.Printf("Projects     %d\n", len(projects))
	}
	if deviceErr != nil {
		fmt.Printf("Devices      unavailable (%v)\n", deviceErr)
	} else {
		fmt.Printf("Devices      %d\n", len(devices))
	}
	return nil
}

func runPair(args []string) error {
	fs := flagSet("mctrl pair")
	openBrowser := fs.Bool("open", false, "open the Pair page in the macOS default browser")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: mctrl pair [--open]")
	}
	if *openBrowser && runtime.GOOS != "darwin" {
		return fmt.Errorf("mctrl pair --open is only supported on macOS")
	}
	stateDir, err := config.EnsureStateDir()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(stateDir)
	if err != nil {
		return err
	}
	if *openBrowser && !isMctrlProcess(readPID(stateDir)) {
		return fmt.Errorf("daemon is not running; run mctrl setup --lan first")
	}
	pairing, err := auth.CreatePairing(stateDir, 10*time.Minute)
	if err != nil {
		return err
	}
	pairURL := advertisedPairURL(cfg) + "#token=" + url.QueryEscape(pairing.Token)
	fmt.Printf("Pairing token: %s\n", pairing.Token)
	fmt.Printf("Expires:       %s\n", pairing.ExpiresAt.Format(time.RFC3339))
	fmt.Printf("Open:          %s\n", pairURL)
	if *openBrowser {
		consoleURL := pairingConsoleURL(cfg, pairing.Token)
		if err := exec.Command("open", consoleURL).Start(); err != nil {
			return fmt.Errorf("open Pair Console: %w", err)
		}
		fmt.Printf("Console:       %s\n", consoleURL)
	}
	if qr, qrErr := qrcode.New(pairURL, qrcode.Medium); qrErr == nil {
		fmt.Println("QR code:")
		fmt.Println(qr.ToSmallString(false))
	}
	if cfg.ListenAddress == "127.0.0.1" || cfg.ListenAddress == "localhost" {
		fmt.Println("Note:          the daemon is loopback-only; use `mctrl setup --lan` before pairing from a phone")
	}
	fmt.Println("Keep this token private; it is single-use and expires automatically.")
	return nil
}

func runDoctor(_ []string) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	fmt.Printf("Platform:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if hostname, hostnameErr := os.Hostname(); hostnameErr == nil {
		fmt.Printf("Hostname:     %s\n", hostname)
	}
	if path, err := exec.LookPath("tmux"); err != nil {
		fmt.Printf("tmux:         MISSING (%v)\n", err)
	} else {
		version, _ := exec.Command(path, "-V").CombinedOutput()
		fmt.Printf("tmux:         %s (%s)\n", path, strings.TrimSpace(string(version)))
	}
	cfg, cfgErr := loadConfig(stateDir)
	if cfgErr != nil {
		fmt.Printf("Config:       MISSING (%v)\n", cfgErr)
	} else {
		fmt.Printf("Config:       %s\n", filepath.Join(stateDir, "config.json"))
		fmt.Printf("Address:      %s\n", cfg.Address())
		fmt.Printf("Transport:    %s\n", transportLabel(cfg.TransportProfile))
		if cfg.PublicURL != "" {
			fmt.Printf("Public URL:   %s\n", cfg.PublicURL)
		}
		fmt.Printf("At login:     %t\n", cfg.StartAfterLogin)
	}
	if pid := readPID(stateDir); pid > 0 && isMctrlProcess(pid) {
		fmt.Printf("Daemon:       running (pid %d)\n", pid)
		if cfgErr == nil {
			client := http.Client{Timeout: 2 * time.Second}
			if response, requestErr := client.Get("http://" + localAddress(cfg) + "/healthz"); requestErr == nil {
				var health struct {
					Status string `json:"status"`
				}
				_ = json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&health)
				_ = response.Body.Close()
				fmt.Printf("API:           reachable (HTTP %d, %s)\n", response.StatusCode, health.Status)
			} else {
				fmt.Printf("API:           unreachable (%v)\n", requestErr)
			}
			state := "off"
			if power.AssertionActiveForOwner(pid) {
				state = "on_ac" // caffeinate -s is AC-aware; policy details stay in config.
			}
			fmt.Printf("Power:         %s (policy %s)\n", state, cfg.RemoteAvailability)
			if works, worksErr := work.NewStore(stateDir).List(); worksErr == nil {
				for _, item := range works {
					if item.State == work.StateStarting || item.State == work.StateRunning {
						fmt.Printf("Work assertion: %t (%s, pid %d)\n", power.WorkAssertionActive(item.RunnerPID), item.ID, item.RunnerPID)
					}
				}
			}
		}
	} else {
		fmt.Println("Daemon:       stopped")
	}
	projects, projectErr := project.NewStore(stateDir).List()
	if projectErr == nil {
		fmt.Printf("Projects:     %d\n", len(projects))
		for _, item := range projects {
			state := "ok"
			if info, err := os.Stat(item.Path); err != nil || !info.IsDir() {
				state = "missing"
			}
			fmt.Printf("  - %s [%s] %s\n", item.Name, state, item.Path)
		}
	}
	for _, detection := range runner.NewRegistry().List() {
		state := "available"
		if !detection.Available {
			state = "missing"
		}
		version := detection.Version
		if version == "" {
			version = "-"
		}
		fmt.Printf("Runner %-8s %s (%s)\n", detection.ID, state, version)
	}
	return nil
}

func runLogs(args []string) error {
	fs := flagSet("logs")
	lines := fs.Int("n", 80, "number of lines")
	if err := fs.Parse(args); err != nil {
		return err
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "logs", "mctrl.log"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("No daemon log yet.")
			return nil
		}
		return err
	}
	all := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if *lines > 0 && len(all) > *lines {
		all = all[len(all)-*lines:]
	}
	fmt.Println(strings.Join(all, "\n"))
	return nil
}

func runProject(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("project requires add, remove, or list")
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	store := project.NewStore(stateDir)
	switch args[0] {
	case "list":
		items, err := store.List()
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Println("No registered Projects.")
			return nil
		}
		for _, item := range items {
			fmt.Printf("%s\t%s\t%s\t%s\n", item.ID, item.Name, item.DefaultRunner, item.Path)
		}
		return nil
	case "add":
		fs := flagSet("project add")
		name := fs.String("name", "", "display name")
		runnerID := fs.String("runner", "shell", "default runner")
		// The documented CLI places the path before optional flags. The
		// standard flag package stops at the first positional argument, so
		// move the path to the end for parsing.
		raw := args[1:]
		pathIndex := -1
		for index, arg := range raw {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if info, statErr := os.Stat(arg); statErr == nil && info.IsDir() {
				pathIndex = index
				break
			}
		}
		if pathIndex < 0 {
			for index, arg := range raw {
				if !strings.HasPrefix(arg, "-") {
					pathIndex = index
					break
				}
			}
		}
		if pathIndex < 0 {
			return fmt.Errorf("usage: mctrl project add <path> [--name <name>] [--runner <id>]")
		}
		path := raw[pathIndex]
		parseArgs := append([]string{}, raw[:pathIndex]...)
		parseArgs = append(parseArgs, raw[pathIndex+1:]...)
		parseArgs = append(parseArgs, path)
		if err := fs.Parse(parseArgs); err != nil {
			return err
		}
		item, err := store.Add(*name, path, *runnerID)
		if err != nil {
			return err
		}
		fmt.Printf("Added %s (%s)\n", item.ID, item.Path)
		return nil
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: mctrl project remove <id>")
		}
		if err := store.Remove(args[1]); err != nil {
			return err
		}
		fmt.Println("Removed", args[1])
		return nil
	default:
		return fmt.Errorf("unknown project command %q", args[0])
	}
}

func runDevices(_ []string) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	devices, err := auth.NewRegistry(stateDir).List()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		fmt.Println("No paired devices.")
		return nil
	}
	for _, device := range devices {
		state := "active"
		if device.RevokedAt != nil {
			state = "revoked"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", device.ID, device.Name, device.CreatedAt.Format(time.RFC3339), state)
	}
	return nil
}

func runRevoke(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: mctrl revoke <device-id>")
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	if err := auth.NewRegistry(stateDir).Revoke(args[0]); err != nil {
		return err
	}
	fmt.Println("Revoked", args[0])
	return nil
}

func runRestart(_ []string) error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(stateDir)
	if err != nil {
		return err
	}
	if launchAgentLoaded() {
		oldPID := readPID(stateDir)
		if err := exec.Command("launchctl", "kickstart", "-k", launchctlServiceTarget()).Run(); err != nil {
			return fmt.Errorf("restart LaunchAgent: %w", err)
		}
		return waitForDaemonRestart(cfg, stateDir, oldPID, 10*time.Second)
	}
	pid := readPID(stateDir)
	if pid > 0 && isMctrlProcess(pid) {
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			return err
		}
		for i := 0; i < 30 && processAlive(pid); i++ {
			time.Sleep(100 * time.Millisecond)
		}
	}
	return startBackground()
}

func runUninstall(args []string) error {
	fs := flagSet("uninstall")
	purge := fs.Bool("purge", false, "delete mctrl state (explicit destructive action)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	if err := removeLaunchAgent(); err != nil {
		return fmt.Errorf("remove LaunchAgent: %w", err)
	}
	pid := readPID(stateDir)
	if pid > 0 && isMctrlProcess(pid) {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	if *purge {
		if err := os.RemoveAll(stateDir); err != nil {
			return err
		}
		fmt.Println("Removed mctrl state:", stateDir)
	} else {
		fmt.Println("Stopped mctrl daemon. Persistent state was preserved at", stateDir)
	}
	return nil
}

func loadConfig(stateDir string) (config.Config, error) {
	return config.Load(filepath.Join(stateDir, "config.json"))
}

func daemonReachable(cfg config.Config) bool {
	client := http.Client{Timeout: 500 * time.Millisecond}
	response, err := client.Get("http://" + localAddress(cfg) + "/healthz")
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func waitForDaemon(cfg config.Config, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if daemonReachable(cfg) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("daemon did not become reachable")
}

func waitForDaemonRestart(cfg config.Config, stateDir string, oldPID int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if daemonReachable(cfg) {
			newPID := readPID(stateDir)
			if oldPID <= 0 || (newPID > 0 && newPID != oldPID) {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("restarted daemon did not become reachable with a new pid")
}

func localAddress(cfg config.Config) string {
	if cfg.ListenAddress == "0.0.0.0" || cfg.ListenAddress == "::" {
		return fmt.Sprintf("127.0.0.1:%d", cfg.Port)
	}
	return cfg.Address()
}

func advertisedPairURL(cfg config.Config) string {
	return config.AdvertisedPairURL(cfg)
}

func pairingConsoleURL(cfg config.Config, token string) string {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	if baseURL == "" {
		baseURL = "http://" + localAddress(cfg)
	}
	return baseURL + "/#/pair-console?token=" + url.QueryEscape(token)
}

func transportLabel(profile config.TransportProfile) string {
	if profile == config.TransportTLSTerminated {
		return "TLS-terminated HTTPS/WSS (configured upstream)"
	}
	return "Trusted LAN HTTP (no end-to-end encryption)"
}

const launchAgentLabel = "com.mctrl.daemon"

func launchctlDomain() string        { return fmt.Sprintf("gui/%d", os.Getuid()) }
func launchctlServiceTarget() string { return launchctlDomain() + "/" + launchAgentLabel }

func launchAgentLoaded() bool {
	return exec.Command("launchctl", "print", launchctlServiceTarget()).Run() == nil
}

func removeLaunchAgent() error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	if launchAgentLoaded() {
		if err := exec.Command("launchctl", "bootout", launchctlDomain(), path).Run(); err != nil {
			return err
		}
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func launchAgentPath() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("LaunchAgent is supported only on macOS")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.mctrl.daemon.plist"), nil
}

func installLaunchAgent() error {
	path, err := launchAgentPath()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	pathEnv := strings.TrimSpace(os.Getenv("PATH"))
	if pathEnv == "" {
		pathEnv = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
	}
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>`+launchAgentLabel+`</string>
  <key>ProgramArguments</key>
  <array><string>%s</string><string>serve</string></array>
  <key>EnvironmentVariables</key>
  <dict>
    <key>MCTRL_HOME</key><string>%s</string>
    <key>PATH</key><string>%s</string>
    <key>TMUX_TMPDIR</key><string>/private/tmp</string>
    <key>LANG</key><string>en_US.UTF-8</string>
    <key>LC_ALL</key><string>en_US.UTF-8</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ProcessType</key><string>Interactive</string>
  <key>StandardOutPath</key><string>%s</string>
  <key>StandardErrorPath</key><string>%s</string>
</dict>
</plist>
`, html.EscapeString(executable), html.EscapeString(stateDir), html.EscapeString(pathEnv), html.EscapeString(filepath.Join(stateDir, "logs", "mctrl.log")), html.EscapeString(filepath.Join(stateDir, "logs", "mctrl.log")))
	if err := os.WriteFile(path, []byte(plist), 0600); err != nil {
		return err
	}
	if output, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		return fmt.Errorf("validate LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	domain := launchctlDomain()
	_ = exec.Command("launchctl", "bootout", domain, path).Run()
	if err := exec.Command("launchctl", "bootstrap", domain, path).Run(); err != nil {
		return err
	}
	return nil
}

func startBackground() error {
	stateDir, err := config.StateDir()
	if err != nil {
		return err
	}
	cfg, err := loadConfig(stateDir)
	if err != nil {
		return err
	}
	if daemonReachable(cfg) {
		return nil
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := filepath.Join(stateDir, "logs", "mctrl.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	command := exec.Command(executable, "serve")
	command.Env = append(os.Environ(), "MCTRL_HOME="+stateDir)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	command.Stdout = logFile
	command.Stderr = logFile
	devNull, devNullErr := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if devNullErr == nil {
		command.Stdin = devNull
	}
	if err := command.Start(); err != nil {
		if devNull != nil {
			_ = devNull.Close()
		}
		_ = logFile.Close()
		return err
	}
	_ = command.Process.Release()
	if devNull != nil {
		_ = devNull.Close()
	}
	_ = logFile.Close()
	return waitForDaemon(cfg, 5*time.Second)
}

func readPID(stateDir string) int {
	data, err := os.ReadFile(filepath.Join(stateDir, "daemon.pid"))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func flagSet(name string) *flag.FlagSet {
	return flag.NewFlagSet(name, flag.ContinueOnError)
}

func newListener(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}
