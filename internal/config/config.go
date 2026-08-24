// Package config loads kimai-cli settings from disk.
package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds everything kimai-cli needs to talk to a Kimai instance.
type Config struct {
	// URL is the base URL of the Kimai instance, without a trailing slash.
	URL string `toml:"url"`
	// Token is the API token, given literally. Prefer TokenCommand.
	Token string `toml:"token"`
	// TokenCommand is a shell command whose stdout is the API token. Using
	// this keeps the secret out of the config file.
	TokenCommand string `toml:"token_command"`
	// DefaultActivity is the activity ID preselected when starting an entry.
	DefaultActivity int `toml:"default_activity"`
	// DefaultProject is the project ID preselected when starting an entry.
	DefaultProject int `toml:"default_project"`
	// StatusFormat is the template used by `status` when --format is omitted.
	StatusFormat string `toml:"status_format"`
	// Interactive makes commands prompt by default, as if -i were given.
	// An explicit --interactive=false still overrides it.
	Interactive bool `toml:"interactive"`
}

// Path returns the location of the config file, honouring KIMAI_CONFIG.
func Path() string {
	if p := os.Getenv("KIMAI_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "kimai-cli", "config.toml")
}

// Load reads the config file and resolves the API token. Use it from
// commands that talk to Kimai; LoadFile is enough to inspect settings.
func Load() (*Config, error) {
	c, err := LoadFile()
	if err != nil {
		return nil, err
	}
	if err := c.resolveToken(); err != nil {
		return nil, err
	}
	return c, nil
}

// LoadFile reads the config file without resolving the API token, so that
// inspecting settings never triggers a decryption or a password prompt.
func LoadFile() (*Config, error) {
	path := Path()
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no config at %s: run `kimai-cli config init`", path)
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	if env := os.Getenv("KIMAI_URL"); env != "" {
		c.URL = env
	}
	c.URL = strings.TrimRight(c.URL, "/")
	if c.URL == "" {
		return nil, fmt.Errorf("no url set in %s", path)
	}
	return &c, nil
}

// resolveToken fills in Token from the environment or token_command.
func (c *Config) resolveToken() error {
	if env := os.Getenv("KIMAI_TOKEN"); env != "" {
		c.Token = env
		return nil
	}
	if c.Token != "" {
		return nil
	}
	if c.TokenCommand == "" {
		return fmt.Errorf("no token: set token_command in %s or export KIMAI_TOKEN", Path())
	}

	out, err := exec.Command("sh", "-c", c.TokenCommand).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return fmt.Errorf("token_command failed: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return fmt.Errorf("token_command failed: %w", err)
	}
	c.Token = strings.TrimSpace(string(out))
	if c.Token == "" {
		return fmt.Errorf("token_command produced no output")
	}
	return nil
}

// Save writes the config to disk, creating parent directories as needed.
func (c *Config) Save() error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}
