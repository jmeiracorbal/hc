package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jmeiracorbal/hybrid-coco/internal/config"
	"github.com/jmeiracorbal/hybrid-coco/internal/doctor"
	"github.com/jmeiracorbal/hybrid-coco/internal/indexer"
	hcpmcp "github.com/jmeiracorbal/hybrid-coco/internal/mcp"
	"github.com/jmeiracorbal/hybrid-coco/internal/migrate"
	"github.com/jmeiracorbal/hybrid-coco/internal/setup"
	"github.com/jmeiracorbal/hybrid-coco/internal/store"
	"github.com/jmeiracorbal/hybrid-coco/internal/upgrade"
)

var version = "dev"

func SetVersion(v string) {
	version = v
	config.SetCoreVersion(v)
}

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "hc",
		Short:         "hybrid-coco — local code intelligence",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.Version = version

	root.AddCommand(cmdIndex())
	root.AddCommand(cmdUpdate())
	root.AddCommand(cmdStatus())
	root.AddCommand(cmdQuery())
	root.AddCommand(cmdSymbol())
	root.AddCommand(cmdFileContext())
	root.AddCommand(cmdPackage())
	root.AddCommand(cmdExplore())
	root.AddCommand(cmdImpact())
	root.AddCommand(cmdPath())
	root.AddCommand(cmdServe())
	root.AddCommand(cmdInit())
	root.AddCommand(cmdReset())
	root.AddCommand(cmdSetup())
	root.AddCommand(cmdMigrate())
	root.AddCommand(cmdDoctor())
	root.AddCommand(cmdUpgrade())
	return root
}

func startPath(args []string) (string, error) {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	return filepath.Abs(root)
}

func cmdIndex() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "index [PATH]",
		Short: "Index project (requires .hc marker from hc init)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := startPath(args)
			if err != nil {
				return err
			}
			root, _, _, err := config.ResolveProject(abs)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Indexing %s …\n", root)
			result, err := indexer.IndexPath(root, force)
			if err != nil {
				return err
			}
			fmt.Printf("Done. Indexed: %d  Skipped (unchanged): %d  Pruned: %d  Errors: %d\n",
				result.Indexed, result.Skipped, result.Pruned, result.Errors)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Re-index all files even if unchanged")
	return cmd
}

func cmdUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update [PATH]",
		Short: "Re-index only changed files (requires .hc)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := startPath(args)
			if err != nil {
				return err
			}
			root, _, dbPath, err := config.ResolveProject(abs)
			if err != nil {
				return err
			}
			if _, err := os.Stat(dbPath); err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("shared index not found at %s. Run: hc setup", dbPath)
				}
				return err
			}
			fmt.Fprintf(os.Stderr, "Updating %s …\n", root)
			result, err := indexer.IndexPath(root, false)
			if err != nil {
				return err
			}
			fmt.Printf("Done. Re-indexed: %d  Unchanged: %d  Pruned: %d  Errors: %d\n",
				result.Indexed, result.Skipped, result.Pruned, result.Errors)
			return nil
		},
	}
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status [PATH]",
		Short: "Show index statistics",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := startPath(args)
			if err != nil {
				return err
			}
			root, id, dbPath, err := config.ResolveProject(abs)
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(root)
			if err != nil {
				return err
			}
			defer st.Close()
			stats, err := st.Stats()
			if err != nil {
				return err
			}
			idShort := id
			if len(idShort) > 12 {
				idShort = idShort[:12]
			}
			marker, err := config.ReadMarker(root)
			if err != nil {
				return err
			}
			fmt.Printf("Root:    %s\nMarker:  %s\nID:      %s…\nhc:      %s\n",
				root, filepath.Join(root, config.MarkerFile), idShort, marker.Version)
			fmt.Println(hcpmcp.FormatStatus(stats, dbPath))
			return nil
		},
	}
}

func cmdQuery() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "query <TEXT>",
		Short: "FTS5 search across symbol names, signatures and docstrings",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return fmt.Errorf("limit must be > 0")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(cwd)
			if err != nil {
				return err
			}
			defer st.Close()
			results, err := st.FTSSearch(args[0], limit)
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("No results.")
				return nil
			}
			for _, r := range results {
				doc := ""
				if r.Docstring != "" {
					snippet := r.Docstring
					if len(snippet) > 80 {
						snippet = snippet[:80]
					}
					doc = " — " + snippet
				}
				fmt.Printf("[%s:%d]  %s %s%s\n", r.Path, r.LineStart, r.Kind, r.Name, doc)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Max results")
	return cmd
}

func cmdSymbol() *cobra.Command {
	return &cobra.Command{
		Use:   "symbol <NAME>",
		Short: "Lookup a symbol by name (exact, then prefix)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(cwd)
			if err != nil {
				return err
			}
			defer st.Close()
			results, err := st.LookupSymbol(args[0])
			if err != nil {
				return err
			}
			enrich, err := hcpmcp.EnrichSymbols(st, results)
			if err != nil {
				return err
			}
			fmt.Println(hcpmcp.FormatSymbol(args[0], results, enrich))
			return nil
		},
	}
}

func cmdExplore() *cobra.Command {
	return &cobra.Command{
		Use:   "explore <NAME>",
		Short: "Structural neighborhood: callers, callees, containment, fan-in/out",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(cwd)
			if err != nil {
				return err
			}
			defer st.Close()
			data, err := st.Explore(args[0])
			if err != nil {
				return err
			}
			text, err := hcpmcp.FormatExplore(args[0], data)
			if err != nil {
				return err
			}
			fmt.Println(text)
			return nil
		},
	}
}

func cmdImpact() *cobra.Command {
	return &cobra.Command{
		Use:   "impact <NAME>",
		Short: "Blast radius: direct callers and file importers",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(cwd)
			if err != nil {
				return err
			}
			defer st.Close()
			data, err := st.Impact(args[0])
			if err != nil {
				return err
			}
			fmt.Println(hcpmcp.FormatImpact(args[0], data))
			return nil
		},
	}
}

func cmdPath() *cobra.Command {
	return &cobra.Command{
		Use:   "path <FROM> <TO>",
		Short: "Call path between two symbols (BFS, max depth 6)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(cwd)
			if err != nil {
				return err
			}
			defer st.Close()
			hops, err := st.Path(args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Println(hcpmcp.FormatPath(args[0], args[1], hops))
			return nil
		},
	}
}

func cmdFileContext() *cobra.Command {
	return &cobra.Command{
		Use:   "file-context <PATH>",
		Short: "Show all symbols in PATH (~97% token savings vs cat)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(cwd)
			if err != nil {
				return err
			}
			defer st.Close()
			data, err := st.FileContext(args[0])
			if err != nil {
				return err
			}
			if data == nil {
				return fmt.Errorf("file '%s' not found in index. Is it indexed? Run: hc update", args[0])
			}
			text, err := hcpmcp.FormatFileContext(args[0], data)
			if err != nil {
				return err
			}
			fmt.Println(text)
			return nil
		},
	}
}

func cmdPackage() *cobra.Command {
	return &cobra.Command{
		Use:   "package <DIR>",
		Short: "Directory/package outline with fan-in and Read range hints",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "" {
				return fmt.Errorf("dir is required")
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			st, err := indexer.RequireStore(cwd)
			if err != nil {
				return err
			}
			defer st.Close()
			data, err := st.PackageOutline(args[0])
			if err != nil {
				return err
			}
			text, err := hcpmcp.FormatPackage(args[0], data)
			if err != nil {
				return err
			}
			fmt.Println(text)
			return nil
		},
	}
}

func cmdServe() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start MCP server (stdio); tools require .hc in the project",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return hcpmcp.Run()
		},
	}
}

func cmdInit() *cobra.Command {
	var globalConfig bool
	cmd := &cobra.Command{
		Use:   "init [PATH]",
		Short: "Create .hc marker, enroll in shared index, and register MCP",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := startPath(args)
			if err != nil {
				return err
			}

			fmt.Println("hybrid-coco init")
			fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			legacy := filepath.Join(abs, config.LegacyHCDir)
			hadLegacy := false
			if st, err := os.Stat(legacy); err == nil && st.IsDir() {
				hadLegacy = true
			}

			canon, id, dbPath, err := config.InitProject(abs)
			if err != nil {
				return err
			}
			if hadLegacy {
				fmt.Println("  ✓ removed legacy .hybrid-coco/")
			}
			fmt.Printf("  ✓ marker %s (hc %s)\n", filepath.Join(canon, config.MarkerFile), config.CoreVersion())

			stEnroll, err := store.Open(dbPath)
			if err != nil {
				return err
			}
			if err := stEnroll.EnrollProject(id, canon); err != nil {
				_ = stEnroll.Close()
				return err
			}
			if err := stEnroll.Close(); err != nil {
				return err
			}

			fmt.Printf("Indexing %s …\n", canon)
			if _, err := indexer.IndexPath(canon, false); err != nil {
				return err
			}
			st, err := indexer.RequireStore(canon)
			if err != nil {
				return err
			}
			stats, err := st.Stats()
			st.Close()
			if err != nil {
				return err
			}
			idShort := id
			if len(idShort) > 12 {
				idShort = idShort[:12]
			}
			fmt.Printf("  ✓ %d files indexed, %d symbols\n", stats.Files, stats.Symbols)
			fmt.Printf("  ✓ Index: %s\n", dbPath)
			fmt.Printf("  ✓ ID: %s…\n\n", idShort)

			var settingsPath string
			var label string
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if globalConfig {
				settingsPath = filepath.Join(home, ".claude", "settings.json")
				label = "~/.claude/settings.json"
			} else {
				settingsPath = filepath.Join(canon, ".claude", "settings.json")
				label = ".claude/settings.json"
			}
			mcpRes, err := setup.RegisterMCP(settingsPath)
			if err != nil {
				return err
			}
			fmt.Printf("MCP server registered in %s\n", label)
			for _, t := range mcpRes.Tools {
				fmt.Printf("  ✓ %s\n", t)
			}
			fmt.Println()

			claudeDir := filepath.Join(home, ".claude")
			g, err := setup.InstallGlobal(claudeDir)
			if err != nil {
				return err
			}
			fmt.Println("Global Claude Code integration")
			fmt.Println("  ✓ ~/.claude/hybrid-coco.md written")
			if g.ClaudeMDUpdated {
				fmt.Println("  ✓ @hybrid-coco.md added to ~/.claude/CLAUDE.md")
			} else {
				fmt.Println("  ✓ @hybrid-coco.md already in ~/.claude/CLAUDE.md")
			}
			fmt.Println("  ✓ Hooks installed in ~/.claude/hooks/")
			if len(g.SkillsInstalled) > 0 {
				fmt.Printf("  ✓ Skills: %s\n", strings.Join(g.SkillsInstalled, ", "))
			}
			fmt.Println("  ✓ PreToolUse: Read|Grep → hc_* (requires .hc)")
			fmt.Println("  ✓ PostToolUse: Write|Edit → hc update")
			fmt.Println()
			fmt.Println("Done. Restart Claude Code to activate.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&globalConfig, "global", false, "Register in ~/.claude/settings.json instead of .claude/settings.json")
	return cmd
}

func cmdReset() *cobra.Command {
	return &cobra.Command{
		Use:   "reset [PATH]",
		Short: "Remove .hc marker and project rows from shared index",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			abs, err := startPath(args)
			if err != nil {
				return err
			}
			root, marker, err := config.FindRoot(abs)
			if err != nil {
				if errors.Is(err, config.ErrMarkerNotFound) {
					_, _, rerr := config.ResetProject(abs)
					if rerr != nil && !errors.Is(rerr, config.ErrMarkerNotFound) {
						return rerr
					}
					fmt.Printf("No .hc marker at %s (legacy cleaned if present).\n", abs)
					return nil
				}
				return err
			}
			dbPath, err := config.SharedIndexPath()
			if err != nil {
				return err
			}
			if _, err := os.Stat(dbPath); err == nil {
				st, err := store.Open(dbPath)
				if err != nil {
					return err
				}
				if err := st.DeleteProject(marker.ID); err != nil {
					_ = st.Close()
					return err
				}
				if err := st.Close(); err != nil {
					return err
				}
			} else if !os.IsNotExist(err) {
				return err
			}
			root, id, err := config.ResetProject(root)
			if err != nil {
				return err
			}
			idShort := id
			if len(idShort) > 12 {
				idShort = idShort[:12]
			}
			fmt.Printf("Reset %s (id %s…): marker and project rows removed (shared DB kept).\n", root, idShort)
			return nil
		},
	}
}

func cmdSetup() *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Install global hooks/skills and create shared index.db",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			claudeDir := filepath.Join(home, ".claude")
			g, err := setup.InstallGlobal(claudeDir)
			if err != nil {
				return err
			}
			fmt.Println("hybrid-coco setup")
			fmt.Println("  ✓ hooks + awareness installed")
			if len(g.SkillsInstalled) > 0 {
				fmt.Printf("  ✓ skills: %s\n", strings.Join(g.SkillsInstalled, ", "))
			}
			if g.ClaudeMDUpdated {
				fmt.Println("  ✓ @hybrid-coco.md added to ~/.claude/CLAUDE.md")
			}
			if g.SharedIndexPath != "" {
				fmt.Printf("  ✓ shared index: %s\n", g.SharedIndexPath)
			}
			fmt.Println("Run 'hc init' inside a project to enroll and index.")
			return nil
		},
	}
}

func cmdMigrate() *cobra.Command {
	var (
		restoreBackup bool
		restorePath   string
		importLegacy  bool
		mapFlags      []string
		purgeLegacy   bool
		yes           bool
	)
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending schema migrations, restore backup, or import legacy indexes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			modes := 0
			if restoreBackup {
				modes++
			}
			if importLegacy {
				modes++
			}
			if modes > 1 {
				return fmt.Errorf("use only one of --restore-backup or --import-legacy")
			}

			dbPath, err := config.SharedIndexPath()
			if err != nil {
				return err
			}

			if restoreBackup {
				if !yes {
					return fmt.Errorf("--restore-backup requires --yes")
				}
				from, err := migrate.RestoreBackup(dbPath, restorePath)
				if err != nil {
					return err
				}
				fmt.Printf("Restored %s from %s\n", dbPath, from)
				return nil
			}

			if importLegacy {
				ids, err := config.ListLegacyIndexIDs()
				if err != nil {
					return err
				}
				if len(mapFlags) == 0 {
					if len(ids) == 0 {
						return fmt.Errorf("no legacy indexes/<id>/index.db found")
					}
					fmt.Println("Legacy index ids (pass --map id=/abs/path to import):")
					for _, id := range ids {
						fmt.Printf("  %s\n", id)
					}
					return fmt.Errorf("--map is required")
				}
				maps, err := migrate.ParseLegacyMaps(mapFlags)
				if err != nil {
					return err
				}
				if purgeLegacy && !yes {
					return fmt.Errorf("--purge-legacy requires --yes")
				}
				res, err := migrate.ImportLegacy(dbPath, maps, purgeLegacy)
				if err != nil {
					return err
				}
				for _, id := range res.Imported {
					fmt.Printf("Imported: %s\n", id)
				}
				for _, id := range res.Skipped {
					fmt.Printf("Skipped:  %s\n", id)
				}
				if res.Purged {
					fmt.Println("Purged indexes/")
				}
				return nil
			}

			if _, err := os.Stat(dbPath); err == nil {
				bak, err := migrate.Backup(dbPath)
				if err != nil {
					return err
				}
				fmt.Printf("Backup: %s\n", bak)
			} else if !os.IsNotExist(err) {
				return err
			}
			db, err := migrate.OpenDB(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			have, err := migrate.ReadVersion(db)
			if err != nil {
				return err
			}
			want := migrate.CurrentSchema()
			pending, _, _, err := migrate.Pending(db)
			if err != nil {
				return err
			}
			fmt.Printf("Schema: have=%d want=%d\n", have, want)
			if len(pending) == 0 {
				fmt.Println("Up to date.")
				return nil
			}
			for _, m := range pending {
				fmt.Printf("  pending: %d %s\n", m.Version, m.Name)
			}
			applied, err := migrate.ApplyPending(db)
			if err != nil {
				return err
			}
			for _, v := range applied {
				fmt.Printf("  applied: %d\n", v)
			}
			fmt.Printf("Done. Schema now %d.\n", migrate.CurrentSchema())
			return nil
		},
	}
	cmd.Flags().BoolVar(&restoreBackup, "restore-backup", false, "Restore shared index from a .bak file")
	cmd.Flags().StringVar(&restorePath, "backup", "", "Backup path for --restore-backup (default: latest .bak)")
	cmd.Flags().BoolVar(&importLegacy, "import-legacy", false, "Import indexes/<id>/ into shared index.db")
	cmd.Flags().StringArrayVar(&mapFlags, "map", nil, "Legacy id to project root (id=/abs/path); repeatable")
	cmd.Flags().BoolVar(&purgeLegacy, "purge-legacy", false, "Remove indexes/ after successful --import-legacy")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm restore or purge-legacy")
	return cmd
}

func cmdDoctor() *cobra.Command {
	var asJSON, fix, yes bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose shared index, marker, and binary freshness",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			report, err := doctor.Run(cwd)
			if err != nil {
				return err
			}
			if fix {
				done, err := doctor.Fix(report, doctor.FixOpts{Yes: yes})
				if err != nil {
					return err
				}
				for _, id := range done {
					fmt.Printf("Fixed: %s\n", id)
				}
				report, err = doctor.Run(cwd)
				if err != nil {
					return err
				}
			}
			if asJSON {
				raw, err := doctor.FormatJSON(report)
				if err != nil {
					return err
				}
				fmt.Println(string(raw))
				return nil
			}
			fmt.Print(doctor.FormatText(report))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON report")
	cmd.Flags().BoolVar(&fix, "fix", false, "Apply safe repairs")
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm destructive repairs (recreate/purge/restore)")
	return cmd
}

func cmdUpgrade() *cobra.Command {
	var yes, doInstall bool
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Check GitHub for a newer hc binary (optionally install)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if doInstall && !yes {
				return fmt.Errorf("--install requires --yes")
			}
			running := config.CoreVersion()
			if running == "" {
				running = version
			}
			fmt.Printf("Running: %s\n", running)

			tag, err := upgrade.LatestRelease(cmd.Context())
			if err != nil {
				fmt.Printf("Latest:  unknown (%v)\n", err)
				if doInstall {
					return fmt.Errorf("cannot install: no GitHub release available: %w", err)
				}
				if yes {
					fmt.Println(upgrade.InstallHint())
				} else {
					fmt.Println("No published release found (or API unreachable). Install via install.sh when a release exists.")
				}
				return nil
			}
			fmt.Printf("Latest:  %s\n", tag)

			outdated := running != "" && running != "dev" && upgrade.CompareVersions(running, tag)
			if running == "" || running == "dev" {
				fmt.Println("Update check skipped for version compare (dev/unset).")
			} else if !outdated {
				fmt.Println("Up to date.")
				if !doInstall {
					return nil
				}
			} else {
				fmt.Println("A newer release is available.")
			}

			if doInstall {
				exe, err := os.Executable()
				if err != nil {
					return err
				}
				exe, err = filepath.EvalSymlinks(exe)
				if err != nil {
					return err
				}
				fmt.Printf("Installing %s → %s\n", tag, exe)
				if err := upgrade.InstallBinary(cmd.Context(), upgrade.InstallOpts{
					Version:  tag,
					DestPath: exe,
				}); err != nil {
					return err
				}
				fmt.Println("Installed. Run: hc doctor")
				return nil
			}
			if yes {
				fmt.Println(upgrade.InstallHint())
				fmt.Println("Or: hc upgrade --install --yes")
			} else if outdated {
				fmt.Println("Re-run with --install --yes to download, or --yes for install.sh hint.")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "Confirm install or print install.sh hint")
	cmd.Flags().BoolVar(&doInstall, "install", false, "Download, verify sha256, replace this binary (requires --yes)")
	return cmd
}
