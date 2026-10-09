package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{Version: 1, Profiles: map[string]Profile{"agency": {Platform: "meta", TokenEnv: "ADSCTL_TEST_TOKEN", APIVersion: "v25.0", Mode: "read"}}}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profiles["agency"].TokenEnv != "ADSCTL_TEST_TOKEN" {
		t.Fatal(got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	t.Setenv("ADSCTL_TEST_TOKEN", "fixture")
	token, err := got.Profiles["agency"].Token()
	if err != nil || token != "fixture" {
		t.Fatal(token, err)
	}
}
func TestConfigRejectsSecretsAndAmbiguousSources(t *testing.T) {
	for _, body := range []string{"version: 1\nprofiles:\n  a:\n    token: secret\n", "version: 1\nprofiles:\n  a:\n    platform: meta\n    token_env: A\n    token_file: /a\n    api_version: v25.0\n    mode: read\n"} {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err == nil {
			t.Fatal("accepted unsafe config")
		}
	}
}
func TestAccountValidation(t *testing.T) {
	for _, id := range []string{"123", "act_123"} {
		v, err := Account(id)
		if err != nil || v != "act_123" {
			t.Fatal(v, err)
		}
	}
	for _, id := range []string{"", "../me", "https://example.com"} {
		if _, err := Account(id); err == nil {
			t.Fatal(id)
		}
	}
}
