package cli

import (
	"sort"

	"adsctl/internal/config"
	"adsctl/internal/output"
	"github.com/spf13/cobra"
)

func (a *App) loadConfig() (config.Config, string, error) {
	path, err := config.Path(a.opts.Config)
	if err != nil {
		return config.Config{}, "", err
	}
	cfg, err := config.Load(path)
	return cfg, path, err
}
func (a *App) addConfig(root *cobra.Command) {
	g := group("config", "Manage non-secret configuration")
	root.AddCommand(g)
	g.AddCommand(action("path", "Print configuration path", "local", func(c *cobra.Command, _ []string) error {
		path, err := config.Path(a.opts.Config)
		if err != nil {
			return err
		}
		return a.emit(c, map[string]any{"path": path})
	}))
	g.AddCommand(action("validate", "Validate configuration references", "local", func(c *cobra.Command, _ []string) error {
		cfg, _, err := a.loadConfig()
		if err != nil {
			return err
		}
		return a.emit(c, map[string]any{"valid": true, "profiles": len(cfg.Profiles)})
	}))
	p := group("profiles", "Manage credential references")
	g.AddCommand(p)
	p.AddCommand(action("list", "List profiles without reading secrets", "local", func(c *cobra.Command, _ []string) error {
		cfg, _, err := a.loadConfig()
		if err != nil {
			return err
		}
		names := []string{}
		for name := range cfg.Profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		rows := []any{}
		for _, n := range names {
			rows = append(rows, map[string]any{"name": n, "profile": cfg.Profiles[n]})
		}
		return a.emit(c, rows)
	}))
	get := action("get NAME", "Read a profile without reading its secret", "local", func(c *cobra.Command, args []string) error {
		cfg, _, err := a.loadConfig()
		if err != nil {
			return err
		}
		v, ok := cfg.Profiles[args[0]]
		if !ok {
			return output.Fail("CONFIG", "Unknown profile.", 2)
		}
		return a.emit(c, v)
	})
	get.Args = cobra.ExactArgs(1)
	p.AddCommand(get)
	var platform, tokenEnv, tokenFile, version, mode, appSecret string
	set := action("set NAME", "Create or update a credential reference", "local", func(c *cobra.Command, args []string) error {
		cfg, path, err := a.loadConfig()
		if err != nil {
			return err
		}
		v := cfg.Profiles[args[0]]
		if v.Platform == "" {
			v = config.Profile{Platform: "meta", APIVersion: "v25.0", Mode: "read"}
		}
		if c.Flags().Changed("token-env") && c.Flags().Changed("token-file") {
			return output.Fail("USAGE", "Choose exactly one token source.", 2)
		}
		if c.Flags().Changed("platform") {
			v.Platform = platform
		}
		if c.Flags().Changed("api-version") {
			v.APIVersion = version
		}
		if c.Flags().Changed("mode") {
			v.Mode = mode
		}
		if c.Flags().Changed("app-secret-env") {
			v.AppSecretEnv = appSecret
		}
		if c.Flags().Changed("token-env") {
			v.TokenEnv = tokenEnv
			v.TokenFile = ""
		}
		if c.Flags().Changed("token-file") {
			v.TokenFile = tokenFile
			v.TokenEnv = ""
		}
		cfg.Profiles[args[0]] = v
		if err = cfg.Save(path); err != nil {
			return err
		}
		return a.emit(c, map[string]any{"name": args[0], "profile": v})
	})
	set.Args = cobra.ExactArgs(1)
	f := set.Flags()
	f.StringVar(&platform, "platform", "meta", "Advertising platform")
	f.StringVar(&tokenEnv, "token-env", "", "Environment variable name, not a token")
	f.StringVar(&tokenFile, "token-file", "", "Raw secret file path")
	f.StringVar(&version, "api-version", "v25.0", "API version")
	f.StringVar(&mode, "mode", "read", "Local read/write mode")
	f.StringVar(&appSecret, "app-secret-env", "", "Optional app-secret environment reference")
	p.AddCommand(set)
	del := action("delete NAME", "Delete a local profile (does not revoke the token)", "local", func(c *cobra.Command, args []string) error {
		cfg, path, err := a.loadConfig()
		if err != nil {
			return err
		}
		if _, ok := cfg.Profiles[args[0]]; !ok {
			return output.Fail("CONFIG", "Unknown profile.", 2)
		}
		delete(cfg.Profiles, args[0])
		if cfg.DefaultProfile == args[0] {
			cfg.DefaultProfile = ""
		}
		if err = cfg.Save(path); err != nil {
			return err
		}
		return a.emit(c, map[string]any{"deleted": args[0]})
	})
	del.Args = cobra.ExactArgs(1)
	p.AddCommand(del)
}
