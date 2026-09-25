package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newZoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zone",
		Short: "DNS zones and the records inside them",
		Long: `Zones are DNS hosting. A zone contains records such as A, CNAME, and TXT.

  cloudns zone list
  cloudns zone list example.com
  cloudns zone info example.com
  cloudns zone add example.com
  cloudns zone types
  cloudns zone soa example.com
  cloudns zone export example.com
  cloudns zone status example.com
  cloudns zone record add example.com CNAME www my-site.pages.dev --ttl 3600
  cloudns zone failover`,
	}
	cmd.AddCommand(
		newZoneListCmd(),
		newZoneInfoCmd(),
		newZoneAddCmd(),
		newZoneDeleteCmd(),
		newZoneTypesCmd(),
		newZoneSOACmd(),
		newZoneExportCmd(),
		newZoneStatusCmd(),
		newZoneMasterCmd(),
		newZoneRecordCmd(),
		newZoneFailoverCmd(),
	)
	return cmd
}

func newZoneListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [zone]",
		Short: "List DNS zones, or the records in one zone",
		Long: `With no zone, list every DNS zone on the account.
With a zone name, list the DNS records in that zone.`,
		Example: `  cloudns zone list
  cloudns zone list example.com
  cloudns zone list example.com --type CNAME --host www`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return runZoneRecords(cmd, args[0])
			}
			return runZoneList(cmd)
		},
	}
	cmd.Flags().String("type", "", "When listing one zone, only this record type")
	cmd.Flags().String("host", "", "When listing one zone, only this host (@ is the apex)")
	return cmd
}

func runZoneList(cmd *cobra.Command) error {
	client, err := sessionClient()
	if err != nil {
		return err
	}
	zones, raw, err := client.ListZones(cmd.Context())
	if err != nil {
		return err
	}
	if wantsJSON(cmd) {
		return printRaw(raw)
	}
	if len(zones) == 0 {
		fmt.Println("No zones.")
		return nil
	}
	rows := make([][]string, len(zones))
	for i, zone := range zones {
		rows[i] = []string{cell(zone.Name), cell(zone.Type), cell(zone.Status)}
	}
	return printTable([]string{"NAME", "TYPE", "STATUS"}, rows)
}

func runZoneRecords(cmd *cobra.Command, zone string) error {
	typ, _ := cmd.Flags().GetString("type")
	host, _ := cmd.Flags().GetString("host")
	client, err := sessionClient()
	if err != nil {
		return err
	}
	recs, raw, err := client.ListRecords(cmd.Context(), zone, typ, host)
	if err != nil {
		return err
	}
	if wantsJSON(cmd) {
		return printRaw(raw)
	}
	if len(recs) == 0 {
		fmt.Printf("No records in %s.\n", zone)
		return nil
	}
	fmt.Println(zone)
	return printRecords(recs)
}

func newZoneRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Add, edit, or delete records in a zone",
	}
	cmd.AddCommand(newZoneRecordAddCmd(), newZoneRecordEditCmd(), newZoneRecordDeleteCmd())
	return cmd
}

func newZoneRecordAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <zone> <type> <host> [value]",
		Short: "Add a DNS record",
		Long: `Add a record in a DNS zone.

The host for the zone apex is @. The value is the address, hostname, text, or URL.
CAA, HINFO, LOC, NAPTR, and RP take flags instead of that value. cloudns zone types
lists a command for every type.

--upsert updates the existing record of the same type and host instead of creating a duplicate.`,
		Example: `  cloudns zone record add example.com CNAME www my-site.pages.dev --ttl 3600
  cloudns zone record add example.com MX @ mail.example.com --priority 10`,
		Args: cobra.RangeArgs(3, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			value := ""
			if len(args) == 4 {
				value = args[3]
			}
			spec, err := specFromRecordArgs(cmd, args[0], args[1], "", args[2], value)
			if err != nil {
				return err
			}
			upsert, _ := cmd.Flags().GetBool("upsert")
			client, err := sessionClient()
			if err != nil {
				return err
			}
			write := client.AddRecord
			if upsert {
				write = client.UpsertRecord
			}
			raw, err := write(cmd.Context(), spec)
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	addRecordFieldFlags(cmd, true)
	cmd.Flags().Bool("upsert", false, "Update the existing record with the same type and host")
	return cmd
}

func newZoneRecordEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <zone> <record-id> <type> <host> [value]",
		Short: "Change a DNS record",
		Long: `Change a record in a DNS zone.

The type must be the record's current type. It is used to choose the fields and
is not sent, because ClouDNS cannot change a record's type. Flags match record add.
cloudns zone types lists a command for every type.`,
		Example: `  cloudns zone record edit example.com 12345 A www 192.0.2.10
  cloudns zone record edit example.com 12345 MX @ mail.example.com --priority 20`,
		Args: cobra.RangeArgs(4, 5),
		RunE: func(cmd *cobra.Command, args []string) error {
			value := ""
			if len(args) == 5 {
				value = args[4]
			}
			spec, err := specFromRecordArgs(cmd, args[0], args[2], args[1], args[3], value)
			if err != nil {
				return err
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.ModifyRecord(cmd.Context(), spec)
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	addRecordFieldFlags(cmd, false)
	return cmd
}

func newZoneRecordDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <zone> <record-id>",
		Short: "Delete a DNS record",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				return errors.New("refusing to delete the record without --yes")
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.DeleteRecord(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	cmd.Flags().BoolP("yes", "y", false, "Confirm deletion")
	return cmd
}
