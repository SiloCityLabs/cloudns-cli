// Package cli is the cloudns command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/ldrrp/cloudns-cli/internal/api"
	"github.com/ldrrp/cloudns-cli/internal/config"
)

const version = "0.1.0"

// Execute runs the cloudns command tree.
func Execute(ctx context.Context) error {
	root := newRoot()
	root.AddCommand(
		newAuthCmd(),
		newZoneCmd(),
		newDomainCmd(),
		newInstallCmd(),
	)
	silence(root)
	return root.ExecuteContext(ctx)
}

func newRoot() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cloudns",
		Short: "ClouDNS HTTP API command-line client",
		Long: `cloudns is a command-line client for the ClouDNS HTTP API.

Zones are DNS hosting and contain records. Domains are names registered at
ClouDNS and have a nameserver delegation. Those are different lists.

Log in once with "cloudns auth login". Later commands read the saved
credentials. CLOUDNS_AUTH_ID, CLOUDNS_SUB_AUTH_ID, CLOUDNS_SUB_AUTH_USER,
and CLOUDNS_AUTH_PASSWORD override those credentials for one invocation
and are never written to disk.`,
		Version: version,
		Example: `  cloudns zone list
  cloudns zone list example.com
  cloudns zone types
  cloudns zone info example.com
  cloudns zone export example.com
  cloudns zone record add example.com CNAME www my-site.pages.dev --ttl 3600
  cloudns domain list
  cloudns domain list example.com
  cloudns domain nameservers set example.com ada.ns.cloudflare.com bob.ns.cloudflare.com`,
		RunE: func(cmd *cobra.Command, args []string) error {
			install, _ := cmd.Flags().GetBool("install")
			if install {
				dir, _ := cmd.Flags().GetString("dir")
				return runInstall(dir)
			}
			return cmd.Help()
		},
	}
	cmd.Flags().Bool("install", false, "Copy this binary onto a directory on PATH")
	cmd.Flags().String("dir", "", "Directory for --install (default: the first writable system bin path)")
	cmd.PersistentFlags().Bool("json", false, "Print the API payload as JSON")
	cmd.CompletionOptions.DisableDefaultCmd = true
	return cmd
}

func silence(cmd *cobra.Command) {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	for _, child := range cmd.Commands() {
		silence(child)
	}
}

func sessionClient() (*api.Client, error) {
	sess, err := config.LoadSession()
	if err != nil {
		return nil, err
	}
	if sess.Warning != "" {
		fmt.Fprintln(os.Stderr, sess.Warning)
	}
	return api.New(sess.Creds), nil
}
