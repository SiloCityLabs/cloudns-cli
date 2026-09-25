package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/ldrrp/cloudns-cli/internal/api"
)

func newZoneFailoverCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "failover",
		Short: "Monitor a DNS record and fail over to a backup IP",
		Long: `DNS failover watches one record and can replace it when the main IP is down.

  cloudns zone failover
  cloudns zone failover nodes
  cloudns zone failover show example.com 12345
  cloudns zone failover add example.com 12345 web --down replace --up activate \
    --main-ip 192.0.2.10 --backup-ip 192.0.2.11 --host www.example.com --port 443

Check types are ping, web, tcp, udp, dns, and smtp.
--down is monitor, deactivate, or replace.
--up is monitor, activate, or ignore.

The account has a limited number of failover checks. cloudns zone failover prints how many are in use.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFailoverUsage(cmd)
		},
	}
	cmd.AddCommand(
		newFailoverNodesCmd(),
		newFailoverShowCmd(),
		newFailoverAddCmd(),
		newFailoverEditCmd(),
		newFailoverDeleteCmd(),
	)
	return cmd
}

func runFailoverUsage(cmd *cobra.Command) error {
	client, err := sessionClient()
	if err != nil {
		return err
	}
	usage, raw, err := client.FailoverUsage(cmd.Context())
	if err != nil {
		return err
	}
	if wantsJSON(cmd) {
		return printRaw(raw)
	}
	fmt.Printf("%s of %s failover checks in use.\n", cell(usage.Count), cell(usage.Limit))
	return nil
}

func newFailoverNodesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "nodes",
		Short: "List failover checker locations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := sessionClient()
			if err != nil {
				return err
			}
			nodes, raw, err := client.FailoverNodes(cmd.Context())
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return printRaw(raw)
			}
			if len(nodes) == 0 {
				fmt.Println("No failover nodes.")
				return nil
			}
			rows := make([][]string, len(nodes))
			for i, node := range nodes {
				rows[i] = []string{cell(node.ID), cell(node.Location), cell(node.IP)}
			}
			return printTable([]string{"ID", "LOCATION", "IP"}, rows)
		},
	}
}

func newFailoverShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <zone> <record-id>",
		Short: "Show the failover settings on a record",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.FailoverSettings(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			if wantsJSON(cmd) {
				return printRaw(raw)
			}
			return printFailoverSettings(raw)
		},
	}
}

func newFailoverAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <zone> <record-id> <type>",
		Short: "Start failover monitoring on a record",
		Example: `  cloudns zone failover add example.com 12345 ping \
    --down monitor --up monitor --main-ip 192.0.2.10 --backup-ip 192.0.2.11`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			check, err := failoverCheckFromFlags(cmd, args[0], args[1], args[2])
			if err != nil {
				return err
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.ActivateFailover(cmd.Context(), check)
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	addFailoverFlags(cmd, false)
	return cmd
}

func newFailoverEditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "edit <zone> <record-id> <type>",
		Short: "Change the failover settings on a record",
		Long: `Change a failover check. ClouDNS replaces the whole configuration, so pass
the type, handlers, main IP, and at least one backup IP again.`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			check, err := failoverCheckFromFlags(cmd, args[0], args[1], args[2])
			if err != nil {
				return err
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.ModifyFailover(cmd.Context(), check)
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	addFailoverFlags(cmd, true)
	return cmd
}

func newFailoverDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <zone> <record-id>",
		Short: "Stop failover monitoring on a record",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			yes, _ := cmd.Flags().GetBool("yes")
			if !yes {
				return errors.New("refusing to delete the failover check without --yes")
			}
			client, err := sessionClient()
			if err != nil {
				return err
			}
			raw, err := client.DeactivateFailover(cmd.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			return printResult(cmd, raw)
		},
	}
	cmd.Flags().Bool("yes", false, "Confirm deletion")
	return cmd
}

func addFailoverFlags(cmd *cobra.Command, edit bool) {
	cmd.Flags().String("down", "", "When the main IP is down: monitor, deactivate, or replace")
	cmd.Flags().String("up", "", "When the main IP is up: monitor, activate, or ignore")
	cmd.Flags().String("main-ip", "", "Main IP address to monitor")
	cmd.Flags().StringArray("backup-ip", nil, "Backup IP (repeat; the first is required, and five is the maximum)")
	cmd.Flags().String("region", "", "Region code, or a node id from zone failover nodes")
	cmd.Flags().String("host", "", "Hostname for web and dns checks")
	cmd.Flags().String("ping-threshold", "", "Ping loss threshold: 15, 25, 50, or 100")
	cmd.Flags().String("protocol", "", "Web protocol: http or https")
	cmd.Flags().String("port", "", "Port for web, tcp, udp, or smtp")
	cmd.Flags().String("path", "", "URL path for a web check")
	cmd.Flags().String("open-port", "", "tcp and udp: 1 if an open port is up, 0 if a closed port is up")
	cmd.Flags().String("content", "", "Response text a web check must return")
	cmd.Flags().String("query-type", "", "DNS record type, such as A")
	cmd.Flags().String("query-response", "", "Expected DNS response")
	cmd.Flags().String("period", "", "Seconds between checks: 60, 120, 300, 600, 900, 1200, 1800, or 3600")
	cmd.Flags().String("mail", "", "Notification address, or -1 to disable mail")
	cmd.Flags().String("deactivate-record", "", "When replace is set and every IP is down: 1 deactivates the record, 0 does not")
	cmd.Flags().String("latency-limit", "", "Ping latency above which the check is down")
	cmd.Flags().String("timeout", "", "Seconds to wait: 1-5 for ping, 4-10 for web")
	cmd.Flags().String("request-type", "", "Web method: GET, HEAD, POST, PUT, or DELETE")
	cmd.Flags().String("header", "", "Custom header name for a web check")
	cmd.Flags().String("header-value", "", "Custom header value for a web check")
	cmd.Flags().String("security", "", "SMTP security: 1 none, 2 STARTTLS, 3 SSL/TLS")
	if edit {
		cmd.Flags().String("state", "", "1 active, or 0 paused")
	}
}

func failoverCheckFromFlags(cmd *cobra.Command, domain, recordID, typ string) (api.FailoverCheck, error) {
	str := func(name string) string {
		v, _ := cmd.Flags().GetString(name)
		return v
	}
	backups, _ := cmd.Flags().GetStringArray("backup-ip")
	check := api.FailoverCheck{
		Domain:             domain,
		RecordID:           recordID,
		Type:               typ,
		Down:               str("down"),
		Up:                 str("up"),
		MainIP:             str("main-ip"),
		Backups:            backups,
		Region:             str("region"),
		Host:               str("host"),
		PingThreshold:      str("ping-threshold"),
		Protocol:           str("protocol"),
		Port:               str("port"),
		Path:               str("path"),
		OpenPort:           str("open-port"),
		Content:            str("content"),
		QueryType:          str("query-type"),
		QueryResponse:      str("query-response"),
		Period:             str("period"),
		Mail:               str("mail"),
		DeactivateRecord:   str("deactivate-record"),
		LatencyLimit:       str("latency-limit"),
		Timeout:            str("timeout"),
		RequestType:        str("request-type"),
		Header:             str("header"),
		HeaderValue:        str("header-value"),
		ConnectionSecurity: str("security"),
	}
	if cmd.Flags().Lookup("state") != nil {
		check.State = str("state")
	}
	return check, nil
}

func printFailoverSettings(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) == 0 {
		return printRaw(raw)
	}
	flat := map[string]string{}
	for key, value := range fields {
		flattenFailover(flat, key, value)
	}
	keys := make([]string, 0, len(flat))
	for key := range flat {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{strings.ToUpper(key), cell(flat[key])})
	}
	return printTable([]string{"FIELD", "VALUE"}, rows)
}

func flattenFailover(out map[string]string, key string, raw json.RawMessage) {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			nested := bytes.TrimSpace([]byte(text))
			if len(nested) > 0 && nested[0] == '{' {
				flattenFailover(out, key, nested)
				return
			}
			out[key] = text
			return
		}
	}
	if len(raw) > 0 && raw[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err == nil {
			for child, value := range obj {
				flattenFailover(out, child, value)
			}
			return
		}
	}
	out[key] = failoverField(raw)
}

func failoverField(raw json.RawMessage) string {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var number json.Number
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&number); err == nil {
		return number.String()
	}
	return strings.TrimSpace(string(raw))
}
