package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureCleanup(t *testing.T) *Cleanup {
	t.Helper()
	l := layoutFor(t.TempDir(), "linux")
	os.MkdirAll(l.Data, 0700)
	os.WriteFile(filepath.Join(l.Data, "ipc.secret"), []byte(randomToken()), 0600)
	c, e := loadCleanup(l)
	if e != nil {
		t.Fatal(e)
	}
	c.state.Port = 43210
	c.startHelper = func() error { return nil }
	return c
}
func cleanupRequest(c *Cleanup, id, token, origin, host string) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]string{"id": id, "token": token})
	r := httptest.NewRequest("POST", c.origin()+"/uninstall", bytes.NewReader(body))
	r.Header.Set("Origin", origin)
	r.Header.Set("Content-Type", "application/json")
	r.Host = host
	w := httptest.NewRecorder()
	c.publicHandler(w, r)
	return w
}
func TestRegistrationPersistenceAndProof(t *testing.T) {
	c := fixtureCleanup(t)
	id, proof := randomToken(), randomToken()
	u, e := c.register(id, proof)
	if e != nil {
		t.Fatal(e)
	}
	again, e := c.register(id, proof)
	if e != nil || u != again {
		t.Fatal("Registration changed")
	}
	if _, e = c.register(id, randomToken()); e == nil {
		t.Fatal("Wrong proof accepted")
	}
	reloaded, e := loadCleanup(c.layout)
	if e != nil || reloaded.state.Profiles[id].Token != c.state.Profiles[id].Token {
		t.Fatal("Registration lost after restart")
	}
}
func TestMultipleProfilesAndReplay(t *testing.T) {
	c := fixtureCleanup(t)
	a, b := randomToken(), randomToken()
	c.register(a, randomToken())
	c.register(b, randomToken())
	token := c.state.Profiles[a].Token
	w := cleanupRequest(c, a, token, c.origin(), "127.0.0.1:43210")
	if w.Code != 200 || c.state.Cleaning || len(c.state.Profiles) != 1 || !strings.Contains(w.Body.String(), "other registered profiles") {
		t.Fatal(w.Code, w.Body.String())
	}
	w = cleanupRequest(c, a, token, c.origin(), "127.0.0.1:43210")
	if w.Code != 403 {
		t.Fatal("Replayed token accepted")
	}
	w = cleanupRequest(c, b, c.state.Profiles[b].Token, c.origin(), "127.0.0.1:43210")
	if w.Code != 200 || !c.state.Cleaning {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, e := c.register(randomToken(), randomToken()); e == nil {
		t.Fatal("Registered during cleanup")
	}
}
func TestUninstallRejectsHostOriginAndToken(t *testing.T) {
	c := fixtureCleanup(t)
	id := randomToken()
	c.register(id, randomToken())
	token := c.state.Profiles[id].Token
	for _, tc := range []struct{ token, origin, host string }{{token, "https://evil.test", "127.0.0.1:43210"}, {token, c.origin(), "evil.test:43210"}, {randomToken(), c.origin(), "127.0.0.1:43210"}, {token, "", "127.0.0.1:43210"}} {
		if w := cleanupRequest(c, id, tc.token, tc.origin, tc.host); w.Code != 403 {
			t.Fatal(w.Code)
		}
	}
	if len(c.state.Profiles) != 1 {
		t.Fatal("Registrations changed")
	}
}
func TestGETNeverDeletes(t *testing.T) {
	c := fixtureCleanup(t)
	id := randomToken()
	c.register(id, randomToken())
	r := httptest.NewRequest("GET", c.origin()+"/uninstall", nil)
	w := httptest.NewRecorder()
	c.publicHandler(w, r)
	if w.Code != 200 || len(c.state.Profiles) != 1 {
		t.Fatal(w.Code)
	}
	for _, h := range []string{"Content-Security-Policy", "Cache-Control", "Referrer-Policy"} {
		if w.Header().Get(h) == "" {
			t.Fatal(h)
		}
	}
}
func TestHelperFailureRestoresRegistration(t *testing.T) {
	c := fixtureCleanup(t)
	id := randomToken()
	c.register(id, randomToken())
	c.startHelper = func() error { return errors.New("unavailable") }
	w := cleanupRequest(c, id, c.state.Profiles[id].Token, c.origin(), "127.0.0.1:43210")
	if w.Code != 500 || len(c.state.Profiles) != 1 || c.state.Cleaning {
		t.Fatal(w.Code)
	}
	loaded, _ := loadCleanup(c.layout)
	if len(loaded.state.Profiles) != 1 {
		t.Fatal("State not restored")
	}
}
func TestIPCAuthentication(t *testing.T) {
	c := fixtureCleanup(t)
	for _, secret := range []string{"", "Bearer invalid", "Bearer " + c.secret} {
		r := httptest.NewRequest("GET", "http://localhost/health", nil)
		r.Header.Set("Authorization", secret)
		w := httptest.NewRecorder()
		c.ipcHandler(w, r)
		want := http.StatusForbidden
		if secret == "Bearer "+c.secret {
			want = 200
		}
		if w.Code != want {
			t.Fatal(w.Code, want)
		}
	}
}
func TestPrivateFiles(t *testing.T) {
	c := fixtureCleanup(t)
	c.register(randomToken(), randomToken())
	s, e := os.Stat(filepath.Join(c.layout.Data, "registrations.json"))
	if e != nil || s.Mode().Perm() != 0600 {
		t.Fatal(s, e)
	}
}
func TestUninstallPreservesUnknownFiles(t *testing.T) {
	c := fixtureCleanup(t)
	os.WriteFile(filepath.Join(c.layout.Data, "unrelated.txt"), []byte("keep"), 0600)
	os.WriteFile(c.layout.Binary, []byte("owned"), 0700)
	run := func(string, ...string) error { return nil }
	if e := uninstall(c.layout, run); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(c.layout.Data, "unrelated.txt")); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(c.layout.Binary); !os.IsNotExist(e) {
		t.Fatal("Binary retained")
	}
	if e := uninstall(c.layout, run); e != nil {
		t.Fatal("Uninstall not idempotent", e)
	}
}
