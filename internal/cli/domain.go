package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ldrrp/cloudns-cli/internal/api"
)

func newDomainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Registered domains and their nameserver delegation",
		Long: `Domains are names registered at ClouDNS. Each domain has nameservers, which
are the delegation at the registrar. That is separate from DNS records inside a zone.

  cloudns domain list
  cloudns domain list example.com
  cloudns domain nameservers set example.com ada.ns.cloudflare.com bob.ns.cloudflare.com`,
	}
	cmd.AddCommand(newDomainListCmd(), newDomainNameserversCmd())
	return cmd
}

func newDomainListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [domain]",
		Short: "List registered domains, or one domain's nameservers",
		Long: `With no domain, list domains registered at ClouDNS.
With a domain name, show the nameservers delegated for that domain.

Those nameservers are not the NS records inside a DNS zone.
Use "zone list <zone>" for the records in a zone.

--sort orders the full domain list. expires and registered put the earliest
date first.`,
		Example: `  cloudns domain list
  cloudns domain list --sort expires
  cloudns domain list example.com`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sortBy, _ := cmd.Flags().GetString("sort")
			if len(args) == 1 {
				if sortBy != "" {
					return fmt.Errorf("--sort applies to the domain list, not one domain's nameservers")
				}
				return runDomainNameservers(cmd, args[0])
			}
			return runDomainList(cmd, sortBy)
		},
	}
	cmd.Flags().String("sort", "", "Sort the domain list by name, expires, or registered")
	return cmd
}

func runDomainList(cmd *cobra.Command, sortBy string) error {
	client, err := sessionClient()
	if err != nil {
		return err
	}
	domains, raw, err := client.ListDomains(cmd.Context())
	if err != nil {
		return err
	}
	domains, raw, err = api.SortDomainPayload(domains, raw, sortBy)
	if err != nil {
		return err
	}
	if wantsJSON(cmd) {
		return printRaw(raw)
	}
	if len(domains) == 0 {
		fmt.Println("No domains.")
		return nil
	}
	rows := make([][]string, len(domains))
	for i, domain := range domains {
		rows[i] = []string{cell(domain.Name), cell(domainStatus(domain.Status)), cell(domain.Expires)}
	}
	return printTable([]string{"NAME", "STATUS", "EXPIRES"}, rows)
}

func runDomainNameservers(cmd *cobra.Command, domain string) error {
	client, err := sessionClient()
	if err != nil {
		return err
	}
	servers, raw, err := client.GetNameservers(cmd.Context(), domain)
	if err != nil {
		return err
	}
	if wantsJSON(cmd) {
		return printRaw(raw)
	}
	fmt.Println(domain)
	if len(servers) == 0 {
		fmt.Println("No nameservers.")
		return nil
	}
	rows := make([][]string, len(servers))
	for i, ns := range servers {
		rows[i] = []string{cell(ns)}
	}
	return printTable([]string{"NAMESERVER"}, rows)
}

func newDomainNameserversCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nameservers",
		Short: "Replace the nameserver delegation for a domain",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "set <domain> <ns> [<ns>...]",
		Short: "Replace the nameservers delegated for a domain",
		Long: `Replace the registrar nameserver delegation for a domain.
This does not edit NS records inside the DNS zone, and it does not append.

Pass the hostnames the DNS host assigned to this zone.
For .de, .be, .ch, .fr, .re, .tf, .wf, .yt, .sh, and .eu, an IPv4 glue
address may follow a hostname.

List your domains first with "cloudns domain list".`,
		Example: `  cloudns domain nameservers set example.com ada.ns.cloudflare.com bob.ns.cloudflare.com
  cloudns domain nameservers set example.be ns1.example.be 203.0.113.10 ns2.example.be 203.0.113.11`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			domain := args[0]
			next, err := api.ParseNameserverArgs(domain, args[1:])
			if err != nil {
				return err
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			prev, _, err := client.GetNameservers(cmd.Context(), domain)
			if err != nil {
				return err
			}
			raw, err := client.SetNameservers(cmd.Context(), domain, next)
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return printRaw(raw)
			}
			printNameserverSet("Previous nameservers", prev)
			fmt.Println()
			printNameserverSet("New nameservers", next)
			return nil
		},
	})
	return cmd
}

func printNameserverSet(title string, servers []string) {
	fmt.Println(title)
	if len(servers) == 0 {
		fmt.Println("  (none)")
		return
	}
	for _, ns := range servers {
		fmt.Printf("  %s\n", ns)
	}
}

func domainStatus(status string) string {
	switch status {
	case "0":
		return "expired"
	case "1":
		return "active"
	case "2":
		return "transfer"
	case "3":
		return "transfer failed"
	default:
		return status
	}
}
