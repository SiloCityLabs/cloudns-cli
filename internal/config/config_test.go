package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSaveUsesKeyringAndMode0600(t *testing.T) {
	ring := &memRing{}
	path := testConfig(t, ring)

	warning, err := Save(Identity{AuthID: "123"}, "disk-secret")
	if err != nil {
		t.Fatal(err)
	}
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	if ring.password() != "disk-secret" {
		t.Fatal("password was not stored in the keyring")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "disk-secret") {
		t.Fatalf("config contains the password: %s", data)
	}
	if !strings.Contains(string(data), `"auth_id": "123"`) {
		t.Fatalf("config = %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("file mode = %o", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode = %o", dirInfo.Mode().Perm())
	}

	sess, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Creds.AuthID != "123" || sess.Creds.Password != "disk-secret" {
		t.Fatalf("session = %+v", sess.Creds)
	}
	if sess.PasswordVia != "system keyring" {
		t.Fatalf("password via = %s", sess.PasswordVia)
	}
}

func TestKeyringFallbackWarnsOnce(t *testing.T) {
	ring := &memRing{setErr: errors.New("keyring unavailable")}
	path := testConfig(t, ring)

	warning, err := Save(Identity{AuthID: "123"}, "disk-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warning, "OS keyring is unavailable") || !strings.Contains(warning, path) {
		t.Fatalf("warning = %q", warning)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "disk-secret") {
		t.Fatalf("password was not stored in the config file: %s", data)
	}

	warning, err = Save(Identity{AuthID: "123"}, "disk-secret")
	if err != nil {
		t.Fatal(err)
	}
	if warning != "" {
		t.Fatalf("second warning = %q", warning)
	}

	sess, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Warning != "" {
		t.Fatalf("load warning = %q", sess.Warning)
	}
	if sess.Creds.Password != "disk-secret" || sess.PasswordVia != "config file" {
		t.Fatalf("session = %+v via %s", sess.Creds, sess.PasswordVia)
	}
}

func TestEnvironmentOverridesAreNotWritten(t *testing.T) {
	ring := &memRing{}
	path := testConfig(t, ring)
	if _, err := Save(Identity{AuthID: "123"}, "disk-secret"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("CLOUDNS_AUTH_ID", "999")
	t.Setenv("CLOUDNS_AUTH_PASSWORD", "env-secret")

	sess, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Creds.AuthID != "999" || sess.Creds.Password != "env-secret" {
		t.Fatalf("session = %+v", sess.Creds)
	}
	if sess.IdentityVia != "environment" || sess.PasswordVia != "environment" {
		t.Fatalf("via identity=%s password=%s", sess.IdentityVia, sess.PasswordVia)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("config changed:\n%s", after)
	}
	if strings.Contains(string(after), "env-secret") || strings.Contains(string(after), "999") {
		t.Fatalf("environment credentials were written: %s", after)
	}
	if ring.password() != "disk-secret" {
		t.Fatal("keyring password changed")
	}
}

func TestLogoutDeletesKeyringAndConfig(t *testing.T) {
	ring := &memRing{}
	path := testConfig(t, ring)
	if _, err := Save(Identity{SubAuthUser: "alice"}, "disk-secret"); err != nil {
		t.Fatal(err)
	}
	if err := Logout(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config file still exists: %v", err)
	}
	if ring.password() != "" {
		t.Fatal("keyring item still exists")
	}
	if err := Logout(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadWarnsOnceForExistingFilePassword(t *testing.T) {
	path := testConfig(t, &memRing{})
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("{\n  \"auth_id\": \"7\",\n  \"password\": \"file-secret\",\n  \"password_store\": \"file\"\n}\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	sess, err := LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Warning == "" || sess.Creds.Password != "file-secret" {
		t.Fatalf("session warning=%q creds=%+v", sess.Warning, sess.Creds)
	}
	sess, err = LoadSession()
	if err != nil {
		t.Fatal(err)
	}
	if sess.Warning != "" {
		t.Fatalf("second warning = %q", sess.Warning)
	}
}

func testConfig(t *testing.T, ring secretStore) string {
	t.Helper()
	for _, key := range []string{
		"CLOUDNS_AUTH_ID",
		"CLOUDNS_SUB_AUTH_ID",
		"CLOUDNS_SUB_AUTH_USER",
		"CLOUDNS_AUTH_PASSWORD",
	} {
		t.Setenv(key, "")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "cloudns", "config.json")
	origPath := configFilePath
	origSecrets := secrets
	configFilePath = func() (string, error) { return path, nil }
	secrets = ring
	t.Cleanup(func() {
		configFilePath = origPath
		secrets = origSecrets
	})
	return path
}

type memRing struct {
	mu     sync.Mutex
	items  map[string]string
	setErr error
}

func (m *memRing) key(service, user string) string {
	return service + "\x00" + user
}

func (m *memRing) Set(service, user, secret string) error {
	if m.setErr != nil {
		return m.setErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items == nil {
		m.items = map[string]string{}
	}
	m.items[m.key(service, user)] = secret
	return nil
}

func (m *memRing) Get(service, user string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	secret, ok := m.items[m.key(service, user)]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return secret, nil
}

func (m *memRing) Delete(service, user string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := m.key(service, user)
	if _, ok := m.items[k]; !ok {
		return keyring.ErrNotFound
	}
	delete(m.items, k)
	return nil
}

func (m *memRing) password() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.items[m.key(keyringService, keyringUser)]
}
