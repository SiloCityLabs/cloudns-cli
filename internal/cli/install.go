package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Copy this binary onto a directory on PATH",
		Long: `Copy the current cloudns executable to the first writable directory among
/usr/local/bin, /usr/bin, and ~/.local/bin. The binary also runs from any path
without being installed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, _ := cmd.Flags().GetString("dir")
			return runInstall(dir)
		},
	}
	cmd.Flags().String("dir", "", "Directory to install into")
	return cmd
}

func runInstall(dir string) error {
	if dir != "" {
		return installTo(dir)
	}
	candidates := binDirs()
	if len(candidates) == 0 {
		return fmt.Errorf("could not find a bin directory; pass --dir")
	}
	var errs []string
	for _, candidate := range candidates {
		err := installTo(candidate)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", candidate, err))
	}
	return fmt.Errorf("could not install cloudns:\n%s", strings.Join(errs, "\n"))
}

func binDirs() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "windows" {
		if home == "" {
			return nil
		}
		return []string{filepath.Join(home, "bin")}
	}
	dirs := []string{"/usr/local/bin", "/usr/bin"}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local", "bin"))
	}
	return dirs
}

func binFileName() string {
	if runtime.GOOS == "windows" {
		return "cloudns.exe"
	}
	return "cloudns"
}

func installTo(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	info, err := os.Stat(exe)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", exe)
	}
	dest := filepath.Join(dir, binFileName())
	if destInfo, err := os.Stat(dest); err == nil && os.SameFile(info, destInfo) {
		fmt.Printf("cloudns is already installed at %s\n", dest)
		return nil
	}
	if err := copyExecutable(exe, dest); err != nil {
		return err
	}
	fmt.Printf("Installed cloudns to %s\n", dest)
	if !dirOnPath(dir) {
		fmt.Fprintf(os.Stderr, "warning: %s is not on PATH\n", dir)
	}
	return nil
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Chmod(0o755); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func dirOnPath(dir string) bool {
	dir = filepath.Clean(dir)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(entry) == dir {
			return true
		}
	}
	return false
}
