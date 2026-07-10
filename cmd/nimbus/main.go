package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/YOUR_USERNAME/nimbus-sync/internal/auth"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/config"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/report"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/scheduler"
	"github.com/YOUR_USERNAME/nimbus-sync/internal/sync"
)

const version = "0.1.0"

func usage() {
	fmt.Fprintf(os.Stderr, `Nimbus Sync %s — sync OneDrive, Google Drive and Dropbox to local folders.

Usage:
  nimbus <command> [flags]

Commands:
  auth <provider>       Authorize a provider (onedrive | gdrive | dropbox)
  sync                  Run a one-shot sync of all enabled remotes
  sync --remote NAME    Sync a single named remote
  daemon                Run in foreground and sync on the configured schedule
  report                Print the latest sync report
  report --html FILE    Export the latest report as HTML
  remotes               List configured remotes and their status
  version               Print version

Flags:
  --config PATH         Config file (default: ~/.config/nimbus-sync/config.yaml)
  --dry-run             Show what would be transferred without writing
  --verbose             Verbose logging

Examples:
  nimbus auth gdrive
  nimbus sync --remote work-onedrive --dry-run
  nimbus daemon
`, version)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config file path")
	dryRun := fs.Bool("dry-run", false, "do not write anything")
	verbose := fs.Bool("verbose", false, "verbose logging")
	remote := fs.String("remote", "", "sync only this remote")
	htmlOut := fs.String("html", "", "write report as HTML to this file")
	_ = fs.Parse(os.Args[2:])

	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("nimbus-sync", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		fatal("loading config: %v", err)
	}
	cfg.Verbose = cfg.Verbose || *verbose

	switch cmd {
	case "auth":
		if fs.NArg() < 1 {
			fatal("usage: nimbus auth <onedrive|gdrive|dropbox>")
		}
		if err := auth.Authorize(cfg, fs.Arg(0)); err != nil {
			fatal("auth failed: %v", err)
		}
		fmt.Println("Authorization saved.")

	case "sync":
		engine := sync.NewEngine(cfg, *dryRun)
		rep, err := engine.Run(*remote)
		if err != nil {
			fatal("sync failed: %v", err)
		}
		rep.PrintSummary(os.Stdout)
		if err := report.Save(cfg, rep); err != nil {
			fatal("saving report: %v", err)
		}

	case "daemon":
		if err := scheduler.Run(cfg); err != nil {
			fatal("daemon error: %v", err)
		}

	case "report":
		rep, err := report.Latest(cfg)
		if err != nil {
			fatal("no report found: %v", err)
		}
		if *htmlOut != "" {
			if err := report.WriteHTML(rep, *htmlOut); err != nil {
				fatal("writing HTML: %v", err)
			}
			fmt.Println("HTML report written to", *htmlOut)
			return
		}
		rep.PrintFull(os.Stdout)

	case "remotes":
		for _, r := range cfg.Remotes {
			state := "enabled"
			if !r.Enabled {
				state = "disabled"
			}
			fmt.Printf("%-20s %-10s %s -> %s\n", r.Name, r.Provider, r.RemotePath, r.LocalPath)
			fmt.Printf("%-20s %s\n", "", state)
		}

	default:
		usage()
		os.Exit(2)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "nimbus: "+format+"\n", args...)
	os.Exit(1)
}
