package xipfax_test

import (
	"os"
	"reflect"
	"testing"

	"github.com/XipHubHQ/xipfax-go/internal/credentials"
)

// TestMain strips every variable shaped like a credential from the environment before a test runs.
func TestMain(m *testing.M) {
	credentials.Strip()
	os.Exit(m.Run())
}

func TestTheEnvironmentHoldsNoCredential(t *testing.T) {
	if left := credentials.Left(); len(left) > 0 {
		t.Fatalf("the tests run with %v in their environment", left)
	}
}

func TestStripTakesAKeyATokenAndTheXipFaxVariablesAndKeepsTheRest(t *testing.T) {
	t.Setenv("XIPFAX_API_KEY", "stand-in")
	t.Setenv("XIPFAX_TO", "+1 202 555 0143")
	t.Setenv("GITHUB_TOKEN", "stand-in")
	t.Setenv("PLAIN_SETTING", "kept")

	removed := credentials.Strip()

	if want := []string{"GITHUB_TOKEN", "XIPFAX_API_KEY", "XIPFAX_TO"}; !reflect.DeepEqual(removed, want) {
		t.Errorf("Strip removed %v, not %v", removed, want)
	}
	if got := os.Getenv("PLAIN_SETTING"); got != "kept" {
		t.Errorf("Strip took PLAIN_SETTING too: it is %q", got)
	}
}
