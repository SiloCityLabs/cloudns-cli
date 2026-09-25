package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ldrrp/cloudns-cli/internal/cli"
)

func main() {
	// Leave Ctrl+C as the normal interrupt so prompts and requests exit immediately.
	if err := cli.Execute(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
