//go:build !windows

package main

import (
	"fmt"
	"os"
)

// The Windows service entry only builds for GOOS=windows. This stub keeps
// cross-platform tooling (go build ./..., go vet ./...) working on other
// platforms without dragging Windows-only dependencies in.
func main() {
	fmt.Fprintln(os.Stderr, "hornet-storage windows service build: this binary targets GOOS=windows only")
	os.Exit(1)
}
