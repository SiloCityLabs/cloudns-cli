package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/ldrrp/cloudns-cli/internal/api"
	"github.com/ldrrp/cloudns-cli/internal/config"
)

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in, show status, or log out",
	}
	cmd.AddCommand(newLoginCmd(), newStatusCmd(), newLogoutCmd())
	return cmd
}

func newLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save ClouDNS API credentials after a successful login check",
		Long: `Prompt for a ClouDNS API auth ID and password, or pass --auth-id and --password-stdin.

The auth ID is the numeric API user ID, not the email you use to sign in.
Create one at https://www.cloudns.net/api-settings/ : add an API user, set a
password, and copy the auth ID from the table after you save.

Press Ctrl+C at either prompt to cancel. Nothing is saved until login succeeds.

Sub-users can pass --sub-auth-id or --sub-auth-user instead of --auth-id.`,
		Args: cobra.NoArgs,
		RunE: runLogin,
	}
	cmd.Flags().String("auth-id", "", "Numeric API user ID from https://www.cloudns.net/api-settings/")
	cmd.Flags().String("sub-auth-id", "", "API sub-user id")
	cmd.Flags().String("sub-auth-user", "", "API sub-user name")
	cmd.Flags().Bool("password-stdin", false, "Read the password from stdin")
	cmd.MarkFlagsMutuallyExclusive("auth-id", "sub-auth-id", "sub-auth-user")
	return cmd
}

func runLogin(cmd *cobra.Command, args []string) error {
	authID, _ := cmd.Flags().GetString("auth-id")
	subID, _ := cmd.Flags().GetString("sub-auth-id")
	subUser, _ := cmd.Flags().GetString("sub-auth-user")
	fromStdin, _ := cmd.Flags().GetBool("password-stdin")

	authID = strings.TrimSpace(authID)
	subID = strings.TrimSpace(subID)
	subUser = strings.TrimSpace(subUser)

	if authID == "" && subID == "" && subUser == "" {
		if fromStdin {
			return errors.New("pass --auth-id, --sub-auth-id, or --sub-auth-user with --password-stdin")
		}
		fmt.Fprintln(os.Stderr, loginIntro)
		var err error
		authID, err = promptLine("Auth ID")
		if err != nil {
			return err
		}
	}

	var password string
	var err error
	if fromStdin {
		password, err = readPasswordStdin()
	} else {
		password, err = promptPassword()
	}
	if err != nil {
		return err
	}

	id := config.Identity{AuthID: authID, SubAuthID: subID, SubAuthUser: subUser}
	creds := api.Credentials{
		AuthID:      id.AuthID,
		SubAuthID:   id.SubAuthID,
		SubAuthUser: id.SubAuthUser,
		Password:    password,
	}
	client := api.New(creds)
	if err := client.Login(cmd.Context()); err != nil {
		return err
	}
	warning, err := config.Save(id, password)
	if err != nil {
		return err
	}
	if warning != "" {
		fmt.Fprintln(os.Stderr, warning)
	}
	label, err := creds.Label()
	if err != nil {
		return err
	}
	fmt.Printf("Logged in as %s.\n", label)
	return nil
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the active ClouDNS credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := config.LoadSession()
			if err != nil {
				return err
			}
			if sess.Warning != "" {
				fmt.Fprintln(os.Stderr, sess.Warning)
			}
			fmt.Println("Logged in")
			if sess.IdentityVia == "environment" {
				fmt.Printf("Account: %s (environment)\n", sess.Account)
			} else {
				fmt.Printf("Account: %s\n", sess.Account)
			}
			switch sess.PasswordVia {
			case "environment":
				fmt.Println("Password: environment")
			case "config file":
				fmt.Println("Password: config file")
			default:
				fmt.Println("Password: system keyring")
			}
			return nil
		},
	}
}

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete the saved credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.Logout(); err != nil {
				return err
			}
			fmt.Println("Logged out.")
			return nil
		},
	}
}

const loginIntro = `The auth ID is the numeric API user ID from ClouDNS, not the email you use to sign in.

Create one at https://www.cloudns.net/api-settings/
Add an API user, set a password, and copy the auth ID from the table after you save.

Press Ctrl+C to cancel.`

func promptLine(label string) (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("stdin is not a terminal; pass --auth-id and --password-stdin")
	}
	fmt.Fprintf(os.Stderr, "%s: ", label)
	// Read one byte at a time so a later password prompt cannot consume a buffered line.
	var line []byte
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			if buf[0] != '\r' {
				line = append(line, buf[0])
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && len(line) > 0 {
				break
			}
			if errors.Is(err, io.EOF) {
				fmt.Fprintln(os.Stderr)
				return "", errors.New("cancelled")
			}
			return "", err
		}
	}
	text := strings.TrimSpace(string(line))
	if text == "" {
		return "", fmt.Errorf("%s is required", label)
	}
	return text, nil
}

func promptPassword() (string, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", errors.New("stdin is not a terminal; pass --password-stdin")
	}
	fmt.Fprintln(os.Stderr, "API user password, the one you set for that API user. Press Ctrl+C to cancel.")
	fmt.Fprint(os.Stderr, "Password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if errors.Is(err, io.EOF) {
		return "", errors.New("cancelled")
	}
	if err != nil {
		return "", err
	}
	if len(pw) == 0 {
		return "", errors.New("password is required")
	}
	return string(pw), nil
}

func readPasswordStdin() (string, error) {
	data, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		return "", err
	}
	pw := strings.TrimRight(string(data), "\r\n")
	if pw == "" {
		return "", errors.New("password is required")
	}
	return pw, nil
}
