// Package cli implements adsctl's Cobra command interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"adsctl/internal/output"
	"github.com/spf13/cobra"
)

type Options struct {
	Config, Profile, Account, APIVersion, Output, Color string
	Timeout, RequestTimeout                             time.Duration
	Verbose                                             bool
}
type App struct {
	baseURL    string
	httpClient *http.Client
	started    bool

	In       io.Reader
	Out, Err io.Writer
	opts     Options
	meta     map[string]any
}

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	a := &App{In: in, Out: out, Err: errOut}
	root := a.Root()
	root.SetArgs(args)
	root.SetIn(in)
	root.SetOut(out)
	root.SetErr(errOut)
	if err := root.ExecuteContext(ctx); err != nil {
		e := output.AsError(err)
		var typed *output.Error
		if !errors.As(err, &typed) && !a.started {
			e = output.Fail("USAGE", err.Error(), 2)
		}
		env := output.New(nil, a.meta)
		env.OK = false
		env.Error = e
		_ = output.Write(errOut, "json", env)
		return e.Exit
	}
	return 0
}
func (a *App) emit(cmd *cobra.Command, data any) error {
	meta := map[string]any{"command": cmd.CommandPath(), "complete": true}
	for k, v := range a.meta {
		meta[k] = v
	}
	return output.Write(a.Out, a.opts.Output, output.New(data, meta))
}
func group(use, short string) *cobra.Command {
	c := &cobra.Command{Use: use, Short: short}
	c.RunE = func(c *cobra.Command, _ []string) error {
		_ = c.Help()
		return output.Fail("USAGE", "Choose a subcommand.", 2)
	}
	return c
}
func action(use, short, effect string, run func(*cobra.Command, []string) error) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, Annotations: map[string]string{"effect": effect}, RunE: run}
}
func (a *App) Root() *cobra.Command {
	root := &cobra.Command{Use: "adsctl", Short: "Manage advertising across platforms with an agent-friendly CLI", SilenceUsage: true, SilenceErrors: true}
	root.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		a.started = true
		if a.opts.Output != "json" && a.opts.Output != "jsonl" && a.opts.Output != "table" {
			return output.Fail("USAGE", "Output must be json, jsonl or table.", 2)
		}
		if a.opts.Timeout <= 0 || a.opts.RequestTimeout <= 0 {
			return output.Fail("USAGE", "Timeouts must be positive.", 2)
		}
		return nil
	}
	f := root.PersistentFlags()
	f.StringVar(&a.opts.Config, "config", "", "Configuration file")
	f.StringVar(&a.opts.Profile, "profile", "", "Credential profile")
	f.StringVar(&a.opts.Account, "account", "", "Ad account ID")
	f.StringVar(&a.opts.APIVersion, "api-version", "", "Meta API version override")
	f.StringVar(&a.opts.Output, "output", "json", "Output: json, jsonl, table")
	f.StringVar(&a.opts.Color, "color", "auto", "Color preference (tables currently use no ANSI color)")
	f.DurationVar(&a.opts.Timeout, "timeout", 2*time.Minute, "Total operation deadline")
	f.DurationVar(&a.opts.RequestTimeout, "request-timeout", 30*time.Second, "Per-request deadline")
	f.BoolVar(&a.opts.Verbose, "verbose", false, "Redacted request diagnostics on stderr")
	root.AddCommand(action("version", "Print build version", "local", func(c *cobra.Command, _ []string) error { return a.emit(c, map[string]any{"version": "dev"}) }))
	completion := action("completion SHELL", "Generate shell completion", "local", func(c *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(a.Out)
		case "zsh":
			return root.GenZshCompletion(a.Out)
		case "fish":
			return root.GenFishCompletion(a.Out, true)
		case "powershell":
			return root.GenPowerShellCompletion(a.Out)
		}
		return output.Fail("USAGE", "Shell must be bash, zsh, fish or powershell.", 2)
	})
	completion.Args = cobra.ExactArgs(1)
	root.AddCommand(completion)
	a.addConfig(root)
	a.addMeta(root)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return output.Fail("USAGE", fmt.Sprint(err), 2) })
	return root
}
