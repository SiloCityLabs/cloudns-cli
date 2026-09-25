package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ldrrp/cloudns-cli/internal/api"
)

func newZoneInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info <zone>",
		Short: "Show whether a zone is on the account, and what kind it is",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := sessionClient()
			if err != nil {
				return err
			}
			info, raw, err := client.GetZoneInfo(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return printRaw(raw)
			}
			return printZoneInfo(info)
		},
	}
}

func printZoneInfo(info map[string]string) error {
	if len(info) == 0 {
		fmt.Println("No zone information.")
		return nil
	}
	preferred := []struct{ key, label string }{
		{"name", "NAME"},
		{"domain", "DOMAIN"},
		{"type", "TYPE"},
		{"zone", "ZONE"},
		{"status", "STATUS"},
		{"serial", "SERIAL"},
	}
	seen := map[string]bool{}
	var rows [][]string
	for _, field := range preferred {
		value, ok := info[field.key]
		if !ok {
			continue
		}
		seen[field.key] = true
		rows = append(rows, []string{field.label, cell(value)})
	}
	var rest []string
	for key := range info {
		if !seen[key] {
			rest = append(rest, key)
		}
	}
	sort.Strings(rest)
	for _, key := range rest {
		rows = append(rows, []string{strings.ToUpper(key), cell(info[key])})
	}
	return printTable([]string{"FIELD", "VALUE"}, rows)
}

func newZoneAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <zone>",
		Short: "Create a DNS zone",
		Long: `Create a DNS zone on the account.

The default type is master. Slave zones need --master-ip. Repeat --ns to choose
the starting nameservers; otherwise ClouDNS adds its defaults.`,
		Example: `  cloudns zone add example.com
  cloudns zone add example.com --type slave --master-ip 192.0.2.10`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			zoneType, _ := cmd.Flags().GetString("type")
			masterIP, _ := cmd.Flags().GetString("master-ip")
			nameservers, _ := cmd.Flags().GetStringArray("ns")
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.RegisterZone(cmd.Context(), args[0], zoneType, masterIP, nameservers)
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	cmd.Flags().String("type", "master", "Zone type: master, slave, parked, or geodns")
	cmd.Flags().String("master-ip", "", "Master server IP, required for --type slave")
	cmd.Flags().StringArray("ns", nil, "Starting nameserver (repeat for more than one)")
	return cmd
}

func newZoneDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <zone>",
		Short: "Delete a DNS zone",
		Long:  `Delete a DNS zone and the records in it. This does not delete a domain registration.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				return errors.New("refusing to delete the zone without --yes")
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.DeleteZone(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	cmd.Flags().Bool("yes", false, "Confirm deletion")
	return cmd
}

func newZoneSOACmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "soa <zone>",
		Short: "Show the SOA for a zone",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := sessionClient()
			if err != nil {
				return err
			}
			soa, raw, err := client.GetSOA(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return printRaw(raw)
			}
			return printTable([]string{"FIELD", "VALUE"}, [][]string{
				{"PRIMARY NS", cell(soa.PrimaryNS)},
				{"ADMIN MAIL", cell(soa.AdminMail)},
				{"REFRESH", cell(soa.Refresh)},
				{"RETRY", cell(soa.Retry)},
				{"EXPIRE", cell(soa.Expire)},
				{"DEFAULT TTL", cell(soa.DefaultTTL)},
			})
		},
	}
	cmd.AddCommand(newZoneSOASetCmd())
	return cmd
}

func newZoneSOASetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <zone>",
		Short: "Change SOA fields",
		Long: `Change one or more SOA fields. Unset fields stay as they are.

ClouDNS requires the full SOA on every change, so this command reads the current
SOA and sends it back with your edits.`,
		Example: `  cloudns zone soa set example.com --admin-mail hostmaster@example.com --default-ttl 3600`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			patch, err := soaPatchFromFlags(cmd)
			if err != nil {
				return err
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.SetSOA(cmd.Context(), args[0], patch)
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	cmd.Flags().String("primary-ns", "", "Primary nameserver hostname")
	cmd.Flags().String("admin-mail", "", "DNS admin email")
	cmd.Flags().String("refresh", "", "Refresh interval in seconds (1200-43200)")
	cmd.Flags().String("retry", "", "Retry interval in seconds (180-2419200)")
	cmd.Flags().String("expire", "", "Expire time in seconds (1209600-2419200)")
	cmd.Flags().String("default-ttl", "", "Default TTL in seconds (60-2419200)")
	return cmd
}

func soaPatchFromFlags(cmd *cobra.Command) (api.SOAPatch, error) {
	var patch api.SOAPatch
	changed := 0
	take := func(name string, dest **string) error {
		if !cmd.Flags().Changed(name) {
			return nil
		}
		value, err := cmd.Flags().GetString(name)
		if err != nil {
			return err
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("--%s is empty", name)
		}
		*dest = &value
		changed++
		return nil
	}
	if err := take("primary-ns", &patch.PrimaryNS); err != nil {
		return patch, err
	}
	if err := take("admin-mail", &patch.AdminMail); err != nil {
		return patch, err
	}
	if err := take("refresh", &patch.Refresh); err != nil {
		return patch, err
	}
	if err := take("retry", &patch.Retry); err != nil {
		return patch, err
	}
	if err := take("expire", &patch.Expire); err != nil {
		return patch, err
	}
	if err := take("default-ttl", &patch.DefaultTTL); err != nil {
		return patch, err
	}
	if changed == 0 {
		return patch, errors.New("set at least one of --primary-ns, --admin-mail, --refresh, --retry, --expire, or --default-ttl")
	}
	return patch, nil
}

func newZoneExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export [zone]",
		Short: "Export a zone as a BIND file",
		Long: `Export a master zone in BIND format.

With a zone name, print the file. With --dir, write <zone>.zone in that directory.
--all exports every zone on the account and requires --dir.`,
		Example: `  cloudns zone export example.com
  cloudns zone export --all --dir ./zones`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			dir, _ := cmd.Flags().GetString("dir")
			if all && len(args) == 1 {
				return errors.New("pass a zone name or --all, not both")
			}
			if all && dir == "" {
				return errors.New("--all requires --dir")
			}
			if !all && len(args) == 0 {
				return errors.New("pass a zone name, or use --all --dir")
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			if all {
				return exportAllZones(cmd, client, dir)
			}
			text, raw, err := client.ExportZone(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return printRaw(raw)
			}
			if dir == "" {
				return printZoneFile(text)
			}
			path, err := writeZoneFile(dir, args[0], text)
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		},
	}
	cmd.Flags().Bool("all", false, "Export every zone on the account")
	cmd.Flags().String("dir", "", "Directory for zone files")
	return cmd
}

func exportAllZones(cmd *cobra.Command, client *api.Client, dir string) error {
	zones, _, err := client.ListZones(cmd.Context())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var failed []string
	for _, zone := range zones {
		text, _, err := client.ExportZone(cmd.Context(), zone.Name)
		if err != nil {
			failed = append(failed, zone.Name+": "+err.Error())
			continue
		}
		path, err := writeZoneFile(dir, zone.Name, text)
		if err != nil {
			failed = append(failed, err.Error())
			continue
		}
		fmt.Println(path)
	}
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, "\n"))
	}
	return nil
}

func writeZoneFile(dir, zone, text string) (string, error) {
	name, err := zoneFileName(zone)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func zoneFileName(zone string) (string, error) {
	zone = strings.TrimSpace(zone)
	if zone == "" || zone == "." || zone == ".." || zone != filepath.Base(zone) {
		return "", fmt.Errorf("refusing to write a zone file for %q", zone)
	}
	return zone + ".zone", nil
}

func printZoneFile(text string) error {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	fmt.Print(text)
	return nil
}

func newZoneStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status <zone> [<zone>...]",
		Short: "Show whether a zone is updated on each nameserver",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := sessionClient()
			if err != nil {
				return err
			}
			type view struct {
				Zone        string          `json:"zone"`
				Updated     bool            `json:"updated"`
				Nameservers json.RawMessage `json:"nameservers"`
			}
			views := make([]view, 0, len(args))
			for _, zone := range args {
				updated, _, err := client.ZoneUpdated(cmd.Context(), zone)
				if err != nil {
					return err
				}
				rows, raw, err := client.UpdateStatus(cmd.Context(), zone)
				if err != nil {
					return err
				}
				views = append(views, view{Zone: zone, Updated: updated, Nameservers: raw})
				if wantsJSON(cmd) {
					continue
				}
				label := "no"
				if updated {
					label = "yes"
				}
				fmt.Printf("%s\nUpdated: %s\n", zone, label)
				if len(rows) == 0 {
					fmt.Println("No nameserver status.")
					continue
				}
				table := make([][]string, len(rows))
				for i, row := range rows {
					state := "no"
					if row.Updated {
						state = "yes"
					}
					table[i] = []string{cell(row.Server), cell(row.IPv4), cell(row.IPv6), state}
				}
				if err := printTable([]string{"SERVER", "IPV4", "IPV6", "UPDATED"}, table); err != nil {
					return err
				}
			}
			if !wantsJSON(cmd) {
				return nil
			}
			if len(views) == 1 {
				raw, err := json.Marshal(views[0])
				if err != nil {
					return err
				}
				return printRaw(raw)
			}
			raw, err := json.Marshal(views)
			if err != nil {
				return err
			}
			return printRaw(raw)
		},
	}
}

func newZoneMasterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "master",
		Short: "Master server IPs for a slave zone",
	}
	cmd.AddCommand(newZoneMasterListCmd(), newZoneMasterAddCmd(), newZoneMasterDeleteCmd())
	return cmd
}

func newZoneMasterListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <zone>",
		Short: "List master server IPs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := sessionClient()
			if err != nil {
				return err
			}
			masters, raw, err := client.ListMasters(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return printRaw(raw)
			}
			if len(masters) == 0 {
				fmt.Println("No master servers.")
				return nil
			}
			rows := make([][]string, len(masters))
			for i, master := range masters {
				rows[i] = []string{cell(master.ID), cell(master.IP)}
			}
			return printTable([]string{"ID", "IP"}, rows)
		},
	}
}

func newZoneMasterAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <zone> <ip>",
		Short: "Add a master server IP",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.AddMaster(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
}

func newZoneMasterDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <zone> <id>",
		Short: "Delete a master server by id",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				return errors.New("refusing to delete the master server without --yes")
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.DeleteMaster(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	cmd.Flags().Bool("yes", false, "Confirm deletion")
	return cmd
}
