package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const extensionID = "ddev-manager@cortier.com"
const hostName = "com.cortier.ddev_manager"
const serviceName = "com.cortier.ddev-manager"

type Layout struct{ Home, OS, Data, Binary, Native, Startup, Socket string }

func layoutFor(home, platform string) Layout {
	l := Layout{Home: home, OS: platform, Data: filepath.Join(home, ".local", "share", "cortier-ddev-manager")}
	if platform == "darwin" {
		l.Data = filepath.Join(home, "Library", "Application Support", "Cortier DDEV Manager")
	}
	l.Binary = filepath.Join(l.Data, "ddev-manager")
	l.Socket = filepath.Join(l.Data, "cleanup.sock")
	if platform == "darwin" {
		l.Native = filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts", hostName+".json")
		l.Startup = filepath.Join(home, "Library", "LaunchAgents", serviceName+".plist")
	} else {
		l.Native = filepath.Join(home, ".mozilla", "native-messaging-hosts", hostName+".json")
		l.Startup = filepath.Join(home, ".config", "systemd", "user", serviceName+".service")
	}
	return l
}
func currentLayout() (Layout, error) {
	h, e := os.UserHomeDir()
	if e != nil {
		return Layout{}, e
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return Layout{}, errors.New("Only Linux and macOS are supported")
	}
	return layoutFor(h, runtime.GOOS), nil
}
func writePrivate(path string, data []byte) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".write-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return writePrivate(path, b)
}
func readConfig(l Layout) (Config, error) {
	var c Config
	b, e := os.ReadFile(filepath.Join(l.Data, "config.json"))
	if e != nil {
		return c, errors.New("Companion is not installed. Run ddev-manager install")
	}
	e = json.Unmarshal(b, &c)
	return c, e
}
func executablePath(p, name string) (string, error) {
	if p == "" {
		v, e := exec.LookPath(name)
		if e != nil {
			return "", fmt.Errorf("%s was not found; install it first or specify its absolute path", name)
		}
		p = v
	}
	if !filepath.IsAbs(p) || filepath.Base(p) != name {
		return "", fmt.Errorf("Expected an absolute path to %s", name)
	}
	s, e := os.Stat(p)
	if e != nil || s.IsDir() || s.Mode()&0111 == 0 {
		return "", fmt.Errorf("%s is not executable", p)
	}
	return p, nil
}
func absoluteExecutablePath(p, label string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("%s is not configured", label)
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("Expected an absolute path to %s", label)
	}
	s, e := os.Stat(p)
	if e != nil || s.IsDir() || s.Mode()&0111 == 0 {
		return "", fmt.Errorf("%s is not executable", p)
	}
	return p, nil
}
func ideExecutablePath(c Config) (string, error) {
	p := c.IDEPath
	if p == "" {
		p = os.Getenv("IDE")
	}
	if p == "" {
		return "", errors.New("IDE executable is not configured; set it in extension Settings or $IDE")
	}
	return absoluteExecutablePath(p, "IDE executable")
}
func xmlEscape(v string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", "\"", "&quot;", "'", "&apos;").Replace(v)
}
func startupContent(l Layout) string {
	if l.OS == "darwin" {
		return `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>` + serviceName + `</string><key>ProgramArguments</key><array><string>` + xmlEscape(l.Binary) + `</string><string>serve</string></array><key>RunAtLoad</key><true/><key>KeepAlive</key><true/><key>EnvironmentVariables</key><dict><key>PATH</key><string>` + xmlEscape(os.Getenv("PATH")) + `</string></dict></dict></plist>`
	}
	escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "%", "%%").Replace(l.Binary)
	return "[Unit]\nDescription=Cortier DDEV Manager cleanup service\n\n[Service]\nExecStart=\"" + escaped + "\" serve\nRestart=on-failure\nRestartSec=3\nUMask=0077\nNoNewPrivileges=true\n\n[Install]\nWantedBy=default.target\n"
}

type SystemCommand func(string, ...string) error

func systemCommand(binary string, args ...string) error {
	cmd := exec.Command(binary, args...)
	if b, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("%s: %w: %s", binary, e, strings.TrimSpace(string(b)))
	}
	return nil
}
func preflight(l Layout, run SystemCommand) error {
	if l.OS == "linux" {
		if e := run("systemctl", "--user", "show-environment"); e != nil {
			return fmt.Errorf("Automatic cleanup requires an available systemd user service manager: %w", e)
		}
	}
	return nil
}
func serviceStart(l Layout, run SystemCommand) error {
	if l.OS == "darwin" {
		domain := fmt.Sprintf("gui/%d", os.Getuid())
		_ = run("launchctl", "bootout", domain+"/"+serviceName)
		return run("launchctl", "bootstrap", domain, l.Startup)
	}
	if e := run("systemctl", "--user", "daemon-reload"); e != nil {
		return e
	}
	if e := run("systemctl", "--user", "enable", serviceName+".service"); e != nil {
		return e
	}
	return run("systemctl", "--user", "restart", serviceName+".service")
}
func serviceStop(l Layout, run SystemCommand) error {
	if l.OS == "darwin" {
		target := fmt.Sprintf("gui/%d/%s", os.Getuid(), serviceName)
		if e := run("launchctl", "bootout", target); e != nil {
			if check := run("launchctl", "print", target); check == nil {
				return e
			}
		}
		return nil
	}
	if e := run("systemctl", "--user", "disable", "--now", serviceName+".service"); e != nil {
		if check := run("systemctl", "--user", "is-active", "--quiet", serviceName+".service"); check == nil {
			return e
		}
	}
	return nil
}
func install(l Layout, source string, c Config, run SystemCommand) error {
	if e := preflight(l, run); e != nil {
		return e
	}
	var e error
	c.DdevPath, e = executablePath(c.DdevPath, "ddev")
	if e != nil {
		return e
	}
	c.GitPath, e = executablePath(c.GitPath, "git")
	if e != nil {
		return e
	}
	if c.IDEPath != "" {
		if c.IDEPath, e = absoluteExecutablePath(c.IDEPath, "IDE executable"); e != nil {
			return e
		}
	}
	if e = os.MkdirAll(l.Data, 0700); e != nil {
		return e
	}
	if e = os.Chmod(l.Data, 0700); e != nil {
		return e
	}
	if source != l.Binary {
		b, e := os.ReadFile(source)
		if e != nil {
			return e
		}
		if e = writePrivate(l.Binary, b); e != nil {
			return e
		}
		if e = os.Chmod(l.Binary, 0700); e != nil {
			return e
		}
	}
	if e = writeJSON(filepath.Join(l.Data, "config.json"), c); e != nil {
		return e
	}
	if _, e = os.Stat(filepath.Join(l.Data, "ipc.secret")); os.IsNotExist(e) {
		if e = writePrivate(filepath.Join(l.Data, "ipc.secret"), []byte(randomToken())); e != nil {
			return e
		}
	}
	// Capture the installer shell's PATH for DDEV custom host commands and helpers.
	if e = writePrivate(filepath.Join(l.Data, "path"), []byte(os.Getenv("PATH"))); e != nil {
		return e
	}
	manifest := map[string]any{"name": hostName, "description": "Cortier DDEV Manager", "path": l.Binary, "type": "stdio", "allowed_extensions": []string{extensionID}}
	if e = writeJSON(l.Native, manifest); e != nil {
		return e
	}
	if e = writePrivate(l.Startup, []byte(startupContent(l))); e != nil {
		return e
	}
	if e = serviceStart(l, run); e != nil {
		return fmt.Errorf("Files installed, but cleanup service could not start. Run doctor or uninstall: %w", e)
	}
	return nil
}
func uninstall(l Layout, run SystemCommand) error {
	// Never remove whole directories: only known, owned installation files.
	if b, e := os.ReadFile(l.Native); e == nil {
		var m struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(b, &m) != nil || m.Path != l.Binary {
			return errors.New("Native manifest belongs to a different installation; refusing removal")
		}
	}
	if b, e := os.ReadFile(l.Startup); e == nil {
		escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "%", "%%").Replace(l.Binary)
		if !strings.Contains(string(b), l.Binary) && !strings.Contains(string(b), xmlEscape(l.Binary)) && !strings.Contains(string(b), escaped) {
			return errors.New("Startup registration belongs to a different installation")
		}
	}
	if e := serviceStop(l, run); e != nil {
		return e
	}
	paths := []string{l.Native, l.Startup, l.Socket, filepath.Join(l.Data, "config.json"), filepath.Join(l.Data, "ipc.secret"), filepath.Join(l.Data, "path"), filepath.Join(l.Data, "registrations.json"), filepath.Join(l.Data, "operations.lock"), filepath.Join(l.Data, "service.lock"), l.Binary}
	for _, p := range paths {
		if e := os.Remove(p); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	_ = os.Remove(l.Data)
	if l.OS == "linux" {
		return run("systemctl", "--user", "daemon-reload")
	}
	return nil
}
func copyExecutable(source, target string) error {
	src, e := os.Open(source)
	if e != nil {
		return e
	}
	defer src.Close()
	dst, e := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
	if e != nil {
		return e
	}
	_, e = io.Copy(dst, src)
	closeErr := dst.Close()
	if e != nil {
		return e
	}
	return closeErr
}
