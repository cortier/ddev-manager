package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	DdevPath string `json:"ddevPath"`
	GitPath  string `json:"gitPath"`
	IDEPath  string `json:"idePath,omitempty"`
}
type Surface struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Root       string `json:"root"`
	Branch     string `json:"branch"`
	Repository string `json:"repository"`
	Status     string `json:"status"`
	URL        string `json:"url"`
	Warning    string `json:"warning,omitempty"`
}
type Service struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	Status string `json:"status,omitempty"`
}
type Project struct {
	Name   string `json:"name"`
	Root   string `json:"approot"`
	Status string `json:"status"`
	URL    string `json:"primary_url"`
}
type Runner func(context.Context, string, string, ...string) ([]byte, error)
type Manager struct {
	config Config
	layout Layout
	run    Runner
	launch Runner
	mu     sync.Mutex
}

func runCommand(ctx context.Context, dir, binary string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	var out, stderr limitedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	// Native hosts inherit Firefox's environment, not an interactive shell.
	cmd.Env = os.Environ()
	cmd.WaitDelay = 3 * time.Second
	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", filepath.Base(binary), err, strings.TrimSpace(stderr.String()))
	}
	return out.Bytes(), nil
}

func launchCommand(_ context.Context, dir, binary string, args ...string) ([]byte, error) {
	cmd := exec.Command(binary, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(binary), err)
	}
	if err := cmd.Process.Release(); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(binary), err)
	}
	return nil, nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.Len() < 4<<20 {
		remain := (4 << 20) - b.Len()
		if len(p) > remain {
			p = p[:remain]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func (m *Manager) ddev(ctx context.Context, dir string, args ...string) ([]byte, error) {
	return m.run(ctx, dir, m.config.DdevPath, args...)
}
func rawJSON(data []byte, target any) error {
	// DDEV emits one JSON log envelope per line; ignore non-data log messages.
	for _, line := range bytes.Split(data, []byte("\n")) {
		var e struct {
			Raw json.RawMessage `json:"raw"`
		}
		if json.Unmarshal(line, &e) == nil && len(e.Raw) > 0 && string(e.Raw) != "null" {
			if err := json.Unmarshal(e.Raw, target); err == nil {
				return nil
			}
		}
	}
	return errors.New("DDEV did not return valid project metadata")
}
func (m *Manager) projects(ctx context.Context) ([]Project, error) {
	b, e := m.ddev(ctx, "", "list", "--json-output")
	if e != nil {
		return nil, e
	}
	var p []Project
	e = rawJSON(b, &p)
	return p, e
}
func (m *Manager) project(ctx context.Context, id string) (Project, error) {
	ps, e := m.projects(ctx)
	if e != nil {
		return Project{}, e
	}
	for _, p := range ps {
		if p.Name == id {
			if !filepath.IsAbs(p.Root) {
				return p, errors.New("DDEV returned a non-absolute project directory")
			}
			s, e := os.Stat(p.Root)
			if e != nil || !s.IsDir() {
				return p, errors.New("Project directory no longer exists")
			}
			return p, nil
		}
	}
	return Project{}, errors.New("Project is no longer registered with DDEV; refresh the list")
}
func (m *Manager) discover(ctx context.Context) ([]Surface, error) {
	ps, e := m.projects(ctx)
	if e != nil {
		return nil, e
	}
	surfaces := make([]Surface, 0, len(ps))
	for _, p := range ps {
		s := Surface{ID: p.Name, Name: p.Name, Root: p.Root, Status: p.Status, URL: p.URL}
		if st, e := os.Stat(p.Root); e != nil || !st.IsDir() {
			s.Warning = "Project directory no longer exists"
			surfaces = append(surfaces, s)
			continue
		}
		branch, e := m.run(ctx, p.Root, m.config.GitPath, "branch", "--show-current")
		if e == nil {
			s.Branch = strings.TrimSpace(string(branch))
		}
		common, e := m.run(ctx, p.Root, m.config.GitPath, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if e == nil {
			s.Repository = filepath.Base(filepath.Dir(strings.TrimSpace(string(common))))
		}
		if s.Repository == "" {
			s.Repository = p.Name
		}
		surfaces = append(surfaces, s)
	}
	return surfaces, nil
}
func validURL(v string) bool {
	u, e := url.Parse(v)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}
func firstURL(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(line)
		if validURL(s) {
			return s
		}
	}
	return ""
}
func (m *Manager) hasURLCommand(p Project) bool {
	_, e := os.Stat(filepath.Join(p.Root, ".ddev", "commands", "host", "url"))
	if e == nil {
		return true
	}
	home, _ := os.UserHomeDir()
	for _, base := range []string{os.Getenv("DDEV_GLOBAL_CONFIG"), filepath.Join(home, ".ddev"), filepath.Join(home, ".config", "ddev")} {
		if base != "" {
			if _, e = os.Stat(filepath.Join(base, "commands", "host", "url")); e == nil {
				return true
			}
		}
	}
	return false
}
func (m *Manager) primary(ctx context.Context, p Project) (string, error) {
	if m.hasURLCommand(p) {
		if b, e := m.ddev(ctx, p.Root, "url"); e == nil {
			if u := firstURL(b); u != "" {
				return u, nil
			}
		}
	}
	if validURL(p.URL) {
		return p.URL, nil
	}
	return "", errors.New("DDEV has no valid HTTP(S) URL for this project")
}
func serviceURL(base string, port int) string {
	u, e := url.Parse(base)
	if e != nil || port < 1 || port > 65535 {
		return ""
	}
	u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(port))
	u.Path = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
func (m *Manager) services(ctx context.Context, p Project) ([]Service, error) {
	b, e := m.ddev(ctx, p.Root, "describe", "--json-output")
	if e != nil {
		return nil, e
	}
	var detail struct {
		Services map[string]struct {
			HTTP   string `json:"http_url"`
			HTTPS  string `json:"https_url"`
			Status string `json:"status"`
		} `json:"services"`
	}
	if e = rawJSON(b, &detail); e != nil {
		return nil, e
	}
	base, e := m.primary(ctx, p)
	if e != nil {
		return nil, e
	}
	found := map[string]Service{}
	for name, s := range detail.Services {
		if name == "web" {
			continue
		}
		u := s.HTTPS
		if !validURL(u) {
			u = s.HTTP
		}
		if validURL(u) {
			label := name
			if name == "webhook-site" {
				label = "webhook.site"
			}
			if name == "buggregator" {
				label = "Buggregator"
			}
			found[name] = Service{ID: name, Name: label, URL: u, Status: s.Status}
		}
	}
	// The effective configuration includes named web ports such as Storybook, even when stopped.
	config, e := m.ddev(ctx, p.Root, "utility", "configyaml", "--full-yaml", "--omit-keys=web_environment")
	if e == nil {
		marker := []byte("# Complete processed project configuration:")
		if i := bytes.Index(config, marker); i >= 0 {
			config = config[i+len(marker):]
		}
		var c struct {
			Ports []struct {
				Name  string `yaml:"name"`
				HTTP  int    `yaml:"http_port"`
				HTTPS int    `yaml:"https_port"`
			} `yaml:"web_extra_exposed_ports"`
		}
		if yaml.Unmarshal(config, &c) == nil {
			for _, v := range c.Ports {
				port := v.HTTPS
				scheme := "https"
				if port == 0 {
					port = v.HTTP
					scheme = "http"
				}
				u := serviceURL(base, port)
				if u == "" {
					continue
				}
				parsed, _ := url.Parse(u)
				parsed.Scheme = scheme
				label := v.Name
				if label == "storybook" {
					label = "Storybook"
				}
				found[v.Name] = Service{ID: v.Name, Name: label, URL: parsed.String()}
			}
		}
	}
	if m.hasURLCommand(p) {
		for _, name := range []string{"buggregator", "storybook"} {
			if s, ok := found[name]; ok {
				if b, e := m.ddev(ctx, p.Root, "url", name); e == nil {
					if u := firstURL(b); u != "" {
						s.URL = u
						found[name] = s
					}
				}
			}
		}
	}
	result := []Service{}
	seen := map[string]bool{base: true}
	names := make([]string, 0, len(found))
	for n := range found {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		s := found[n]
		if !seen[s.URL] {
			seen[s.URL] = true
			result = append(result, s)
		}
	}
	return result, nil
}
func (m *Manager) withOperation(fn func() (any, error)) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Cross-process lock also serializes commands from separate Firefox profiles.
	f, e := os.OpenFile(filepath.Join(m.layout.Data, "operations.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return nil, e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if data, err := os.ReadFile(filepath.Join(m.layout.Data, "registrations.json")); err == nil {
		var registry Registry
		if json.Unmarshal(data, &registry) != nil {
			return nil, errors.New("Invalid cleanup registration state; run doctor")
		}
		if registry.Cleaning {
			return nil, errors.New("Companion removal is in progress")
		}
	}
	return fn()
}
func (m *Manager) action(ctx context.Context, id, action, service string) (any, error) {
	return m.withOperation(func() (any, error) {
		p, e := m.project(ctx, id)
		if e != nil {
			return nil, e
		}
		if action == "ide" {
			ide, e := ideExecutablePath(m.config)
			if e != nil {
				return nil, e
			}
			launch := m.launch
			if launch == nil {
				launch = m.run
			}
			if _, e = launch(ctx, p.Root, ide, p.Root); e != nil {
				return nil, e
			}
			return map[string]bool{"ok": true}, nil
		}
		command := ""
		switch action {
		case "start":
			if p.Status != "running" {
				command = "start"
			}
		case "restart":
			command = "restart"
			if p.Status != "running" {
				command = "start"
			}
		case "stop":
			if p.Status != "stopped" {
				command = "stop"
			}
		case "open":
			if p.Status != "running" {
				command = "start"
			}
		default:
			return nil, errors.New("Unsupported lifecycle action")
		}
		if command != "" {
			if _, e = m.ddev(ctx, p.Root, command); e != nil {
				return nil, e
			}
		}
		if action != "open" {
			return map[string]bool{"ok": true}, nil
		}
		p, e = m.project(ctx, id)
		if e != nil {
			return nil, e
		}
		if p.Status != "running" {
			return nil, errors.New("Project did not become ready after startup")
		}
		if service != "" {
			ss, e := m.services(ctx, p)
			if e != nil {
				return nil, e
			}
			for _, s := range ss {
				if s.ID == service {
					return map[string]string{"url": s.URL}, nil
				}
			}
			return nil, errors.New("Service is no longer configured")
		}
		u, e := m.primary(ctx, p)
		return map[string]string{"url": u}, e
	})
}
