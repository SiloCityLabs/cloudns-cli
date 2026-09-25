package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/ldrrp/cloudns-cli/internal/api"
)

func wantsJSON(cmd *cobra.Command) bool {
	v, err := cmd.Flags().GetBool("json")
	return err == nil && v
}

func printRaw(raw json.RawMessage) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		fmt.Println("null")
		return nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return fmt.Errorf("format JSON: %w", err)
	}
	fmt.Println(buf.String())
	return nil
}

func printResult(cmd *cobra.Command, raw json.RawMessage) error {
	if wantsJSON(cmd) {
		return printRaw(raw)
	}
	var st struct {
		Status            string `json:"status"`
		StatusDescription string `json:"statusDescription"`
	}
	if err := json.Unmarshal(raw, &st); err == nil && strings.TrimSpace(st.StatusDescription) != "" {
		fmt.Println(st.StatusDescription)
		return nil
	}
	return printRaw(raw)
}

func printTable(headers []string, rows [][]string) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	return w.Flush()
}

func cell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func displayHost(host string) string {
	if strings.TrimSpace(host) == "" {
		return "@"
	}
	return host
}

func printRecords(recs []api.Record) error {
	rows := make([][]string, len(recs))
	for i, rec := range recs {
		rows[i] = []string{
			cell(rec.ID),
			cell(rec.Type),
			cell(displayHost(rec.Host)),
			cell(rec.Value),
			cell(rec.TTL),
		}
	}
	return printTable([]string{"ID", "TYPE", "HOST", "VALUE", "TTL"}, rows)
}
