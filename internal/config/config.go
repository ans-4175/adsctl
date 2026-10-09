// Package config resolves non-secret profiles and external secret references.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"adsctl/internal/output"
	"go.yaml.in/yaml/v3"
)

type Profile struct {
	Platform     string `yaml:"platform" json:"platform"`
	TokenEnv     string `yaml:"token_env,omitempty" json:"token_env,omitempty"`
	TokenFile    string `yaml:"token_file,omitempty" json:"token_file,omitempty"`
	AppSecretEnv string `yaml:"app_secret_env,omitempty" json:"app_secret_env,omitempty"`
	APIVersion   string `yaml:"api_version" json:"api_version"`
	Mode         string `yaml:"mode" json:"mode"`
}
type Member struct {
	Profile string `yaml:"profile" json:"profile"`
	Account string `yaml:"account" json:"account"`
}
type Config struct {
	Version        int                 `yaml:"version" json:"version"`
	DefaultProfile string              `yaml:"default_profile,omitempty" json:"default_profile,omitempty"`
	Profiles       map[string]Profile  `yaml:"profiles" json:"profiles"`
	Portfolios     map[string][]Member `yaml:"portfolios,omitempty" json:"portfolios,omitempty"`
}

var versionRE = regexp.MustCompile(`^v[1-9][0-9]*\.0$`)
var nameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
var idRE = regexp.MustCompile(`^[0-9]+$`)

func Account(id string) (string, error) {
	id = strings.TrimPrefix(id, "act_")
	if !idRE.MatchString(id) {
		return "", output.Fail("USAGE", "Account must be numeric or act_<numeric ID>.", 2)
	}
	return "act_" + id, nil
}
func ID(id string) error {
	if !idRE.MatchString(id) {
		return output.Fail("USAGE", "Resource IDs must be numeric strings.", 2)
	}
	return nil
}
func ValidVersion(v string) bool { return versionRE.MatchString(v) }
func Path(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "adsctl", "config.yaml"), nil
}
func Load(path string) (Config, error) {
	cfg := Config{Version: 1, Profiles: map[string]Profile{}, Portfolios: map[string][]Member{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, output.Fail("CONFIG", "Cannot read configuration file.", 2)
	}
	if len(b) > 1024*1024 {
		return cfg, output.Fail("CONFIG", "Configuration exceeds 1 MiB.", 2)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err = dec.Decode(&cfg); err != nil {
		return cfg, output.Fail("CONFIG", "Invalid configuration YAML or unknown configuration field.", 2)
	}
	var extra any
	if err = dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return cfg, output.Fail("CONFIG", "Configuration must contain one document.", 2)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	return cfg, cfg.Validate()
}
func (c Config) Validate() error {
	if c.Version != 1 {
		return output.Fail("CONFIG", "Unsupported configuration version.", 2)
	}
	for n, p := range c.Profiles {
		if !nameRE.MatchString(n) || p.Platform != "meta" || !ValidVersion(p.APIVersion) || (p.Mode != "read" && p.Mode != "write") || (p.TokenEnv == "") == (p.TokenFile == "") {
			return output.Fail("CONFIG", fmt.Sprintf("Invalid profile %q: require meta platform, API version, read/write mode and exactly one token reference.", n), 2)
		}
	}
	if c.DefaultProfile != "" {
		if _, ok := c.Profiles[c.DefaultProfile]; !ok {
			return output.Fail("CONFIG", "Default profile does not exist.", 2)
		}
	}
	for name, members := range c.Portfolios {
		if !nameRE.MatchString(name) || len(members) == 0 {
			return output.Fail("CONFIG", "Portfolio requires a valid name and members.", 2)
		}
		for _, m := range members {
			if _, ok := c.Profiles[m.Profile]; !ok {
				return output.Fail("CONFIG", "Portfolio references an unknown profile.", 2)
			}
			if _, err := Account(m.Account); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c Config) Save(path string) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return Atomic(path, b)
}
func Atomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".adsctl-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() { _ = os.Remove(temp) }()
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
func (p Profile) Token() (string, error) {
	var token string
	if p.TokenEnv != "" {
		token = os.Getenv(p.TokenEnv)
		if token == "" {
			return "", output.Fail("CONFIG", "Missing token environment variable: "+p.TokenEnv, 2)
		}
	} else {
		b, err := os.ReadFile(p.TokenFile)
		if err != nil {
			return "", output.Fail("CONFIG", "Cannot read token file for selected profile.", 2)
		}
		if len(b) > 64*1024 {
			return "", output.Fail("CONFIG", "Token file exceeds 64 KiB.", 2)
		}
		token = string(b)
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, "\r\n \t") {
		return "", output.Fail("CONFIG", "Token reference must contain one nonempty token.", 2)
	}
	return token, nil
}
