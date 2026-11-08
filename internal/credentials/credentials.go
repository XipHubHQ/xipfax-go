// Package credentials keeps every credential out of the tests. Each test package's TestMain calls
// Strip before any test runs: anything in the environment shaped like a credential, or named for
// XipFax, goes. The day a real key sits in a developer's shell, a test that could reach the API
// still has nothing to reach it with, and the tests keep saying so.
package credentials

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// secret names a variable that is a secret in itself, in any case.
var secret = regexp.MustCompile(`(?i)(TOKEN|SECRET|PASSWORD|CREDENTIAL|AUTH|APIKEY|API_KEY|_KEY$|_KEYS$|COOKIE|PRIVATE)`)

// xipfax names the variables XipFax's own examples read, such as XIPFAX_API_KEY.
var xipfax = regexp.MustCompile(`(?i)^XIPFAX_`)

// Shaped reports whether a test process has no business carrying the variable name.
func Shaped(name string) bool {
	return secret.MatchString(name) || xipfax.MatchString(name)
}

// Left answers the name of each such variable in the process's environment, sorted.
func Left() []string {
	var left []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if Shaped(name) {
			left = append(left, name)
		}
	}
	sort.Strings(left)
	return left
}

// Strip removes each such variable from the process's environment, answering the names it removed,
// sorted.
func Strip() []string {
	removed := Left()
	for _, name := range removed {
		os.Unsetenv(name)
	}
	return removed
}
