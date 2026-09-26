// growctl declaratively manages GrowSCADA tags from YAML manifests, kubectl-style.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kipitix/growscada/internal/growctl"
)

func main() {
	err := growctl.NewRootCommand().Execute()
	if err == nil {
		return
	}
	var exitErr *growctl.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.Err != nil {
			fmt.Fprintln(os.Stderr, "error:", exitErr.Err)
		}
		os.Exit(exitErr.Code)
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
