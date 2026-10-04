package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/jmeiracorbal/hybrid-coco/internal/coco"
	"github.com/jmeiracorbal/hybrid-coco/internal/coco/install"
)

func cmdCoco() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "coco",
		Short: "Manage language cocos (wasm/v1 plugins)",
	}
	cmd.AddCommand(cmdCocoInstall())
	cmd.AddCommand(cmdCocoList())
	cmd.AddCommand(cmdCocoUninstall())
	return cmd
}

func cmdCocoInstall() *cobra.Command {
	var replace bool
	var local string
	cmd := &cobra.Command{
		Use:   "install [github.com/org/repo@vX.Y.Z]",
		Short: "Install a coco from GitHub Releases or --local DIR",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			var (
				res install.Result
				err error
			)
			if local != "" {
				res, err = install.FromLocal(ctx, local, replace)
			} else {
				if len(args) != 1 {
					return fmt.Errorf("module@version is required (or use --local)")
				}
				res, err = install.FromGitHub(ctx, args[0], replace)
			}
			if err != nil {
				return err
			}
			fmt.Printf("Installed %s (language=%s version=%s)\n  %s\n", res.ID, res.Language, res.Version, res.Dir)
			return nil
		},
	}
	cmd.Flags().BoolVar(&replace, "replace", false, "Replace existing coco with the same id")
	cmd.Flags().StringVar(&local, "local", "", "Install from a local coco directory")
	return cmd
}

func cmdCocoList() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List built-in and installed cocos",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tLANGUAGE\tPRIORITY\tSOURCE\tVERSION")
			builtins, err := coco.Builtins()
			if err != nil {
				return err
			}
			for _, c := range builtins {
				fmt.Fprintf(w, "%s\t%s\t%d\tbuiltin\t-\n", c.ID(), c.Language(), c.Priority())
			}
			recs, err := coco.ListInstalled()
			if err != nil {
				return err
			}
			for _, r := range recs {
				fmt.Fprintf(w, "%s\t%s\t%d\twasm\t%s\n", r.ID, r.Language, r.Priority, r.Version)
			}
			return w.Flush()
		},
	}
}

func cmdCocoUninstall() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall ID",
		Short: "Uninstall an installed coco (e.g. hc/java)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := install.Uninstall(args[0]); err != nil {
				return err
			}
			fmt.Printf("Uninstalled %s\n", args[0])
			return nil
		},
	}
}
