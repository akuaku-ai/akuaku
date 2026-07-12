// Package updater self-updates Akuaku by reinstalling the latest build, so users
// don't have to remember the `go install` incantation. The actual install is
// injected, keeping this package free of subprocess and network concerns.
package updater

import (
	"fmt"
	"io"
)

// Module is the package path `akuaku update` reinstalls.
const Module = "github.com/akuaku-ai/akuaku/cmd/akuaku@latest"

// Run reinstalls Akuaku via install and reports progress to out. install returns
// the combined command output and an error; on failure that output is folded
// into the returned error so the user sees why it failed.
func Run(install func() ([]byte, error), out io.Writer) error {
	fmt.Fprintln(out, "updating akuaku…")
	output, err := install()
	if err != nil {
		return fmt.Errorf("update failed: %w\n%s", err, output)
	}
	fmt.Fprintln(out, "akuaku is up to date — restart any running monitor to pick it up")
	return nil
}
