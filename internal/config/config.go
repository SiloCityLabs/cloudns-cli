// Package config stores the ClouDNS auth id and password.
// The auth id lives in ~/.config/cloudns/config.json (mode 0600).
// The password lives in the OS keyring. If the keyring is unavailable, the
// password is stored in that same file and a warning is returned once.
// Environment variables override a single run and are never written.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"

	"github.com/ldrrp/cloudns-cli/internal/api"
)

const (
	keyringService = "cloudns"
	keyringUser    = "default"

	storeKeyring = "keyring"
	storeFile    = "file"
)

// ErrNotLoggedIn means neither the config file nor the environment has a full login.
var ErrNotLoggedIn = errors.New("not logged in; run `cloudns auth login`")

// Identity is the non-secret account selector saved at login.
type Identity struct {
	AuthID      string
	SubAuthID   string
	SubAuthUser string
}

// Session is the credentials for one invocation, plus where they came from.
type Session struct {
	Creds       api.Credentials
	Account     string
	IdentityVia string
	PasswordVia string
	Warning     string
}

type fileConfig struct {
	AuthID        string `json:"auth_id,omitempty"`
	SubAuthID     string `json:"sub_auth_id,omitempty"`
	SubAuthUser   string `json:"sub_auth_user,omitempty"`
	Password      string `json:"password,omitempty"`
	PasswordStore string `json:"password_store,omitempty"`
	KeyringWarned bool   `json:"keyring_warned,omitempty"`
}

type secretStore interface {
	Set(service, user, secret string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

type zalandoStore struct{}

func (zalandoStore) Set(service, user, secret string) error {
	return keyring.Set(service, user, secret)
}

func (zalandoStore) Get(service, user string) (string, error) {
	return keyring.Get(service, user)
}

func (zalandoStore) Delete(service, user string) error {
	return keyring.Delete(service, user)
}

var secrets secretStore = zalandoStore{}

var configFilePath = defaultConfigPath

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cloudns", "config.json"), nil
}

// Save verifies nothing itself. The caller must check login before saving.
// The password argument is what gets stored. Environment variables are ignored.
func Save(id Identity, password string) (warning string, err error) {
	id.AuthID = strings.TrimSpace(id.AuthID)
	id.SubAuthID = strings.TrimSpace(id.SubAuthID)
	id.SubAuthUser = strings.TrimSpace(id.SubAuthUser)
	n := 0
	if id.AuthID != "" {
		n++
	}
	if id.SubAuthID != "" {
		n++
	}
	if id.SubAuthUser != "" {
		n++
	}
	if n != 1 {
		return "", errors.New("set exactly one of auth-id, sub-auth-id, or sub-auth-user")
	}
	if password == "" {
		return "", errors.New("password is required")
	}

	prev, readErr := readFile()
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}

	cfg := fileConfig{
		AuthID:      id.AuthID,
		SubAuthID:   id.SubAuthID,
		SubAuthUser: id.SubAuthUser,
	}
	if err := secrets.Set(keyringService, keyringUser, password); err != nil {
		cfg.Password = password
		cfg.PasswordStore = storeFile
		cfg.KeyringWarned = true
		if !prev.KeyringWarned {
			path, pathErr := configFilePath()
			if pathErr != nil {
				return "", pathErr
			}
			warning = fmt.Sprintf("warning: OS keyring is unavailable; password stored in %s", path)
		}
	} else {
		cfg.PasswordStore = storeKeyring
	}
	if err := writeFile(cfg); err != nil {
		if cfg.PasswordStore == storeKeyring {
			_ = secrets.Delete(keyringService, keyringUser)
		}
		return "", err
	}
	return warning, nil
}

// LoadSession merges the config file, the keyring, and environment overrides.
// Overrides are applied in memory only.
func LoadSession() (Session, error) {
	cfg, err := readFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Session{}, err
	}

	creds := api.Credentials{
		AuthID:      cfg.AuthID,
		SubAuthID:   cfg.SubAuthID,
		SubAuthUser: cfg.SubAuthUser,
	}
	passwordVia := ""
	switch cfg.PasswordStore {
	case storeFile:
		creds.Password = cfg.Password
		passwordVia = "config file"
	case storeKeyring:
		pw, err := secrets.Get(keyringService, keyringUser)
		if err != nil {
			if errors.Is(err, keyring.ErrNotFound) {
				return Session{}, errors.New("password not found in the keyring; run `cloudns auth login`")
			}
			return Session{}, fmt.Errorf("read password from OS keyring: %s", scrub(err.Error(), cfg.Password))
		}
		creds.Password = pw
		passwordVia = "system keyring"
	}

	var warning string
	if cfg.PasswordStore == storeFile && !cfg.KeyringWarned && creds.Password != "" {
		path, pathErr := configFilePath()
		if pathErr != nil {
			return Session{}, pathErr
		}
		warning = fmt.Sprintf("warning: OS keyring is unavailable; password stored in %s", path)
		cfg.KeyringWarned = true
		if err := writeFile(cfg); err != nil {
			return Session{}, err
		}
	}

	var identityVia string
	creds, identityVia, passwordVia = overlayEnv(creds, passwordVia)
	if creds.AuthID == "" && creds.SubAuthID == "" && creds.SubAuthUser == "" && creds.Password == "" {
		return Session{}, ErrNotLoggedIn
	}
	label, err := creds.Label()
	if err != nil {
		return Session{}, ErrNotLoggedIn
	}
	if creds.Password == "" {
		return Session{}, errors.New("password not found; run `cloudns auth login`")
	}
	if identityVia == "" {
		identityVia = "config"
	}
	return Session{
		Creds:       creds,
		Account:     label,
		IdentityVia: identityVia,
		PasswordVia: passwordVia,
		Warning:     warning,
	}, nil
}

// Logout deletes the keyring item and the config file.
func Logout() error {
	cfg, err := readFile()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	keyErr := secrets.Delete(keyringService, keyringUser)
	if errors.Is(keyErr, keyring.ErrNotFound) {
		keyErr = nil
	}
	path, err := configFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if cfg.PasswordStore == storeKeyring && keyErr != nil {
		return fmt.Errorf("delete keyring item: %s", scrub(keyErr.Error(), cfg.Password))
	}
	return nil
}

func overlayEnv(creds api.Credentials, passwordVia string) (api.Credentials, string, string) {
	identityVia := "config"
	user, userOK := os.LookupEnv("CLOUDNS_SUB_AUTH_USER")
	sub, subOK := os.LookupEnv("CLOUDNS_SUB_AUTH_ID")
	id, idOK := os.LookupEnv("CLOUDNS_AUTH_ID")
	if (userOK && user != "") || (subOK && sub != "") || (idOK && id != "") {
		identityVia = "environment"
		creds.AuthID, creds.SubAuthID, creds.SubAuthUser = "", "", ""
		if idOK && id != "" {
			creds.AuthID = id
		}
		if subOK && sub != "" {
			creds.SubAuthID = sub
		}
		if userOK && user != "" {
			creds.SubAuthUser = user
		}
	}
	if pw, ok := os.LookupEnv("CLOUDNS_AUTH_PASSWORD"); ok && pw != "" {
		creds.Password = pw
		passwordVia = "environment"
	}
	return creds, identityVia, passwordVia
}

func readFile() (fileConfig, error) {
	path, err := configFilePath()
	if err != nil {
		return fileConfig{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileConfig{}, err
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fileConfig{}, fmt.Errorf("read config: %w", err)
	}
	return cfg, nil
}

func writeFile(cfg fileConfig) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	success := false
	defer func() {
		if !success {
			os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	success = true
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	return nil
}

func scrub(msg, secret string) string {
	if secret == "" {
		return msg
	}
	return strings.ReplaceAll(msg, secret, "[redacted]")
}
