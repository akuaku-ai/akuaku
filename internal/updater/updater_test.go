package updater

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRun_SuccessReportsUpToDate(t *testing.T) {
	var out bytes.Buffer
	install := func() ([]byte, error) { return []byte("go: downloading …"), nil }

	if err := Run(install, &out); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("expected a success message, got:\n%s", out.String())
	}
}

func TestRun_FailureWrapsErrorAndOutput(t *testing.T) {
	install := func() ([]byte, error) {
		return []byte("no required module provides package"), errors.New("exit status 1")
	}

	err := Run(install, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected an error when the install fails")
	}
	if !strings.Contains(err.Error(), "no required module provides package") {
		t.Errorf("error should include the install output, got: %v", err)
	}
}
