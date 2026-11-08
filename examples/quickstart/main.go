// Command quickstart runs XipFax's quickstart in your terminal, from a clone of this repository:
//
//	XIPFAX_API_KEY=xipfax_key_live_… go run ./examples/quickstart
//
// That takes the free steps alone. To send a fax too, real and charged, name a fax number of yours
// and a public link to a PDF:
//
//	XIPFAX_TO='+1 202 555 0143' XIPFAX_PDF_URL=https://example.com/letter.pdf XIPFAX_API_KEY=… go run ./examples/quickstart
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/XipHubHQ/xipfax-go"
)

func main() {
	client, err := xipfax.New(os.Getenv("XIPFAX_API_KEY"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Set XIPFAX_API_KEY to a key you made in the developer console: https://developer.xipfax.com/console/")
		os.Exit(1)
	}
	to, pdfURL := os.Getenv("XIPFAX_TO"), os.Getenv("XIPFAX_PDF_URL")
	terminal := bufio.NewReader(os.Stdin)

	err = quickstart(context.Background(), client, input{
		to:     to,
		pdfURL: pdfURL,
		say:    func(line string) { fmt.Println(line) },
		ask: func(question string) string {
			fmt.Print(question)
			// A terminal that closes answers what was typed before it did, which sends only if
			// that was send.
			line, _ := terminal.ReadString('\n')
			return line
		},
		pause: time.Sleep,
	})

	var refused *xipfax.GenericOpenAPIError
	if errors.As(err, &refused) {
		if refusal, ok := refused.Model().(xipfax.APIError); ok {
			fmt.Fprintf(os.Stderr, "XipFax refused it, %s: %s\n", refusal.Error.Code, refusal.Error.Message)
			os.Exit(1)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if to == "" || pdfURL == "" {
		fmt.Println("Those steps cost nothing. To send a fax too, run it again with XIPFAX_TO, a fax number of yours, and XIPFAX_PDF_URL, a public link to a PDF.")
	}
}
