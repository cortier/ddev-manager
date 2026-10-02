package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Registration struct {
	Proof string `json:"proof"`
	Token string `json:"token"`
}
type Registry struct {
	Port     int                     `json:"port"`
	Profiles map[string]Registration `json:"profiles"`
	Cleaning bool                    `json:"cleaning"`
}
type Cleanup struct {
	mu          sync.Mutex
	layout      Layout
	state       Registry
	secret      string
	startHelper func() error
}

func randomToken() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func tokenValid(v string) bool { b, e := hex.DecodeString(v); return e == nil && len(b) == 32 }
func sameSecret(a, b string) bool {
	return len(a) > 0 && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
func loadCleanup(l Layout) (*Cleanup, error) {
	s, e := os.ReadFile(filepath.Join(l.Data, "ipc.secret"))
	if e != nil {
		return nil, e
	}
	c := &Cleanup{layout: l, secret: string(s), state: Registry{Profiles: map[string]Registration{}}}
	b, e := os.ReadFile(filepath.Join(l.Data, "registrations.json"))
	if e == nil {
		if e = json.Unmarshal(b, &c.state); e != nil {
			return nil, e
		}
	} else if !os.IsNotExist(e) {
		return nil, e
	}
	if c.state.Profiles == nil {
		c.state.Profiles = map[string]Registration{}
	}
	c.startHelper = func() error { return spawnCleanupHelper(l) }
	return c, nil
}
func (c *Cleanup) save() error {
	return writeJSON(filepath.Join(c.layout.Data, "registrations.json"), c.state)
}
func (c *Cleanup) origin() string { return fmt.Sprintf("http://127.0.0.1:%d", c.state.Port) }
func (c *Cleanup) register(id, proof string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !tokenValid(id) || !tokenValid(proof) {
		return "", errors.New("Invalid profile registration")
	}
	if c.state.Cleaning {
		return "", errors.New("Companion cleanup is underway; reinstall after it finishes")
	}
	r, ok := c.state.Profiles[id]
	if ok && !sameSecret(r.Proof, proof) {
		return "", errors.New("Profile proof did not match")
	}
	if !ok {
		r = Registration{Proof: proof, Token: randomToken()}
		c.state.Profiles[id] = r
		if e := c.save(); e != nil {
			delete(c.state.Profiles, id)
			return "", e
		}
	}
	// URL fragments stay out of HTTP request targets, proxy logs, and Referer headers.
	return c.origin() + "/uninstall#" + id + ":" + r.Token, nil
}
func secureHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
}
func (c *Cleanup) publicHandler(w http.ResponseWriter, r *http.Request) {
	secureHeaders(w)
	if r.Host != strings.TrimPrefix(c.origin(), "http://") {
		http.Error(w, "Invalid host", http.StatusForbidden)
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/uninstall":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>DDEV Manager removal</title><link rel="stylesheet" href="/cleanup.css"><main><h1>DDEV Manager</h1><p id="result" role="status">Completing removal…</p><p>Your DDEV projects are not changed.</p></main><script src="/cleanup.js"></script></html>`)
	case r.Method == "GET" && r.URL.Path == "/cleanup.css":
		w.Header().Set("Content-Type", "text/css")
		io.WriteString(w, `html{color-scheme:light dark;font:16px system-ui}main{max-width:38rem;margin:12vh auto;padding:2rem}h1{font-size:24px}p{line-height:1.6}`)
	case r.Method == "GET" && r.URL.Path == "/cleanup.js":
		w.Header().Set("Content-Type", "text/javascript")
		io.WriteString(w, `(async()=>{const [id,token]=location.hash.slice(1).split(':');history.replaceState(null,'','/uninstall');const el=document.getElementById('result');try{if(!id||!token)throw Error('This removal link is missing or expired. Use the standalone uninstaller.');const r=await fetch('/uninstall',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({id,token})});const data=await r.json();if(!r.ok)throw Error(data.error);el.textContent=data.message;}catch(e){el.textContent=e.message+' If removal did not finish, run ddev-manager uninstall.'}})();`)
	case r.Method == "POST" && r.URL.Path == "/uninstall":
		if r.Header.Get("Origin") != c.origin() || r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "Invalid origin or content type", http.StatusForbidden)
			return
		}
		var body struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		}
		if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&body); e != nil {
			replyJSON(w, 400, map[string]string{"error": "Invalid cleanup request"})
			return
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		reg, ok := c.state.Profiles[body.ID]
		if !ok || !sameSecret(reg.Token, body.Token) {
			replyJSON(w, 403, map[string]string{"error": "This removal link is invalid or already used"})
			return
		}
		delete(c.state.Profiles, body.ID)
		last := len(c.state.Profiles) == 0
		c.state.Cleaning = last
		if e := c.save(); e != nil {
			c.state.Profiles[body.ID] = reg
			c.state.Cleaning = false
			replyJSON(w, 500, map[string]string{"error": "Could not save cleanup state"})
			return
		}
		if last {
			if e := c.startHelper(); e != nil {
				c.state.Profiles[body.ID] = reg
				c.state.Cleaning = false
				_ = c.save()
				replyJSON(w, 500, map[string]string{"error": "Could not start the cleanup helper; use the standalone uninstaller"})
				return
			}
			replyJSON(w, 200, map[string]string{"message": "Your extension registration was removed. Companion cleanup has started and will finish shortly."})
		} else {
			replyJSON(w, 200, map[string]string{"message": "This Firefox profile was unregistered. The companion remains installed for your other registered profiles."})
		}
	default:
		http.NotFound(w, r)
	}
}
func replyJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (c *Cleanup) ipcHandler(w http.ResponseWriter, r *http.Request) {
	if !sameSecret(r.Header.Get("Authorization"), "Bearer "+c.secret) {
		http.Error(w, "Unauthorized", 403)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/health" {
		replyJSON(w, 200, map[string]any{"ok": true, "version": 1})
		return
	}
	if r.Method != "POST" || r.URL.Path != "/register" {
		http.NotFound(w, r)
		return
	}
	var b struct {
		ID    string `json:"id"`
		Proof string `json:"proof"`
	}
	if e := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&b); e != nil {
		http.Error(w, "Invalid request", 400)
		return
	}
	u, e := c.register(b.ID, b.Proof)
	if e != nil {
		replyJSON(w, 400, map[string]string{"error": e.Error()})
		return
	}
	replyJSON(w, 200, map[string]string{"url": u})
}
func serve(l Layout) error {
	lock, e := os.OpenFile(filepath.Join(l.Data, "service.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.New("Cleanup service is already running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	c, e := loadCleanup(l)
	if e != nil {
		return e
	}
	listener, e := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", c.state.Port))
	if e != nil {
		return fmt.Errorf("Cleanup port unavailable: %w", e)
	}
	defer listener.Close()
	c.state.Port = listener.Addr().(*net.TCPAddr).Port
	if e = c.save(); e != nil {
		return e
	}
	if c.state.Cleaning {
		if e = c.startHelper(); e != nil {
			return e
		}
	}
	_ = os.Remove(l.Socket)
	unix, e := net.Listen("unix", l.Socket)
	if e != nil {
		return e
	}
	defer unix.Close()
	if e = os.Chmod(l.Socket, 0600); e != nil {
		return e
	}
	local := &http.Server{Handler: http.HandlerFunc(c.ipcHandler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 4096}
	go func() { _ = local.Serve(unix) }()
	defer local.Close()
	web := &http.Server{Handler: http.HandlerFunc(c.publicHandler), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 4096}
	return web.Serve(listener)
}
func ipc(l Layout, method, path, body string) (map[string]any, error) {
	secret, e := os.ReadFile(filepath.Join(l.Data, "ipc.secret"))
	if e != nil {
		return nil, e
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", l.Socket)
	}}}
	defer client.CloseIdleConnections()
	req, e := http.NewRequest(method, "http://localhost"+path, strings.NewReader(body))
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+string(secret))
	req.Header.Set("Content-Type", "application/json")
	resp, e := client.Do(req)
	if e != nil {
		return nil, errors.New("Cleanup service is unavailable. Run ddev-manager doctor")
	}
	defer resp.Body.Close()
	var result map[string]any
	if e = json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&result); e != nil {
		return nil, e
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%v", result["error"])
	}
	return result, nil
}
func spawnCleanupHelper(l Layout) error {
	dir, e := os.MkdirTemp("", "ddev-manager-cleanup-")
	if e != nil {
		return e
	}
	target := filepath.Join(dir, "cleanup")
	if e = copyExecutable(l.Binary, target); e != nil {
		os.RemoveAll(dir)
		return e
	}
	var cmd *exec.Cmd
	if l.OS == "linux" {
		cmd = exec.Command("systemd-run", "--user", "--collect", "--unit="+serviceName+"-cleanup", "--property=Type=exec", target, "cleanup-helper")
	} else {
		cmd = exec.Command("launchctl", "submit", "-l", serviceName+"-cleanup", "--", target, "cleanup-helper")
	}
	if b, e := cmd.CombinedOutput(); e != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("Cleanup helper: %w: %s", e, b)
	}
	return nil
}
func cleanupHelper(l Layout) error {
	// The independent service survives stopping the parent cleanup service.
	time.Sleep(3 * time.Second)
	c, e := loadCleanup(l)
	if e != nil {
		return e
	}
	if !c.state.Cleaning || len(c.state.Profiles) != 0 {
		return errors.New("Profiles remain registered; refusing automatic cleanup")
	}
	// Wait for commands already in progress before removing the installation.
	lock, e := os.OpenFile(filepath.Join(l.Data, "operations.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	e = uninstall(l, systemCommand)
	exe, _ := os.Executable()
	if filepath.Base(exe) == "cleanup" && strings.HasPrefix(filepath.Base(filepath.Dir(exe)), "ddev-manager-cleanup-") {
		_ = os.Remove(exe)
		_ = os.Remove(filepath.Dir(exe))
	}
	if l.OS == "darwin" && e == nil {
		_ = systemCommand("launchctl", "remove", serviceName+"-cleanup")
	}
	return e
}
