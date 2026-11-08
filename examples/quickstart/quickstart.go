package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/XipHubHQ/xipfax-go"
)

// XipFax's quickstart through the SDK, the steps of
// https://developer.xipfax.com/docs/guides/quickstart/ in their order. The free steps come first: a
// quote, and the offices that take a form. Then, only where you name a fax number of yours and a
// PDF, a real fax, charged, which it says before it sends, and sends only once you type send.
// main.go runs it in your terminal.

// input is what the quickstart is given, and how it talks to you.
type input struct {
	// to is a fax number of yours, where the fax goes. Without it, only the free steps run.
	to string
	// pdfURL is a public https link to a PDF, the fax's pages. Without it, only the free steps run.
	pdfURL string
	// say tells you a line.
	say func(line string)
	// ask asks you a question, answering what you typed.
	ask func(question string) string
	// pause waits between reads of the fax.
	pause func(time.Duration)
}

// How long the quickstart waits between reads of the fax, and how many it makes.
const (
	readEvery = 10 * time.Second
	reads     = 30
)

// realAndCharged is what the step that sends says first, as the docs' quickstart says it.
const realAndCharged = "This step sends a real fax, and it’s charged. It goes to the number you name, so name one of your own. Its pages count on your plan, or where you have no plan its price is held on the card you saved, and taken once it’s delivered. A fax that fails costs nothing."

// quickstart takes the quickstart's steps. The API's refusal stops it, answered as its error, a
// *xipfax.GenericOpenAPIError whose Model is the API's own, which main.go prints.
func quickstart(ctx context.Context, client *xipfax.APIClient, in input) error {
	// 1. What a fax costs. This costs nothing, and it shows your key works.
	quote, _, err := client.QuoteAPI.GetQuote(ctx).Pages(1).Execute()
	if err != nil {
		return err
	}
	in.say(fmt.Sprintf("A one-page fax costs %s without a plan: %s", quote.PriceShown, quote.Terms))

	// 2. Where a form goes: the offices that take Form 2848 from Ohio, and their numbers. Also free.
	offices, _, err := client.OfficesAPI.ListOffices(ctx).Search("Form 2848").Place("OH").Limit(3).Execute()
	if err != nil {
		return err
	}
	for _, office := range offices.Data {
		in.say(fmt.Sprintf("%s takes Form 2848 at %s.", office.Name, office.FaxNumber))
	}

	if in.to == "" || in.pdfURL == "" {
		return nil
	}

	// 3. Your draft, the fax you prepare. This sends only its own document, so it starts empty.
	before, _, err := client.DraftAPI.GetDraft(ctx).Execute()
	if err != nil {
		return err
	}
	if len(before.Documents) > 0 {
		in.say("Your draft already holds documents. Send or remove them on xipfax.com first, so that this sends only its own.")
		return nil
	}

	// 4. Add the PDF by its link. Each POST names a key of its own, so a retry can't add it twice.
	key, err := newKey()
	if err != nil {
		return err
	}
	added, _, err := client.DraftAPI.AddDocument(ctx).
		IdempotencyKey(key).
		NewDocument(xipfax.NewDocument{Name: nameOf(in.pdfURL), URL: xipfax.PtrString(in.pdfURL)}).
		Execute()
	if err != nil {
		return err
	}
	in.say(fmt.Sprintf("Added %s%s.", added.Document.Name, pagesOf(added.Document)))

	// 5. Send it, a real fax, charged: said first, and sent only once you type send.
	in.say(realAndCharged)
	answer := in.ask(fmt.Sprintf("Type send to fax it to %s, or anything else to stop: ", in.to))
	if strings.ToLower(strings.TrimSpace(answer)) != "send" {
		if _, err := client.DraftAPI.RemoveDocument(ctx, added.Document.ID).Execute(); err != nil {
			return err
		}
		in.say("Nothing was sent, and the document is out of your draft again.")
		return nil
	}
	// The send names the documents to send: the one this added, alone. A draft whose documents
	// aren't these is refused, so that nothing you didn't name is sent.
	if key, err = newKey(); err != nil {
		return err
	}
	sent, _, err := client.DraftAPI.SendDraft(ctx).
		IdempotencyKey(key).
		SendDraft(xipfax.SendDraft{FaxNumber: xipfax.PtrString(in.to), DocumentIDs: []string{added.Document.ID}}).
		Execute()
	if err != nil {
		return err
	}
	if confirmURL := sent.GetConfirmURL(); confirmURL != "" {
		in.say("Nothing was sent or held: the fax waits for you on xipfax.com, " + confirmURL)
		return nil
	}
	in.say(fmt.Sprintf("Sent, paid by %s: fax %s.", sent.GetPaidBy(), sent.Fax.ID))

	// 6. Follow the fax until it's delivered or fails, then read its confirmation.
	fax, shown := sent.Fax, ""
	for read := 0; read < reads && fax.Status != "delivered" && fax.Status != "failed"; read++ {
		in.pause(readEvery)
		now, _, err := client.FaxesAPI.GetFax(ctx, fax.ID).Execute()
		if err != nil {
			return err
		}
		fax = *now
		stands := fax.Status
		if step := fax.GetStep(); step != "" {
			stands += ": " + step
		}
		if stands != shown {
			in.say(stands)
		}
		shown = stands
	}
	switch fax.Status {
	case "delivered":
		confirmation, _, err := client.FaxesAPI.GetConfirmation(ctx, fax.ID).Execute()
		if err != nil {
			return err
		}
		in.say("Its confirmation, readable for a few minutes: " + confirmation.URL)
	case "failed":
		in.say(fmt.Sprintf("It failed, %s, so it costs nothing.", fax.GetFailure()))
	default:
		in.say(fmt.Sprintf("It's still %s. Read it later: client.FaxesAPI.GetFax(ctx, %q).Execute().", fax.Status, fax.ID))
	}
	return nil
}

// newKey answers a key of a request's own for its Idempotency-Key, a random UUID.
func newKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40 // version 4, random
	b[8] = b[8]&0x3f | 0x80 // the variant RFC 9562 gives
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// nameOf answers the PDF's name, as its link ends: letter.pdf.
func nameOf(link string) string {
	name := "document.pdf"
	if parsed, err := url.Parse(link); err == nil {
		segments := strings.FieldsFunc(parsed.EscapedPath(), func(r rune) bool { return r == '/' })
		if len(segments) > 0 {
			name = segments[len(segments)-1]
			// A link whose name isn't percent-encoded text keeps it as it is.
			if text, err := url.PathUnescape(name); err == nil && utf8.ValidString(text) {
				name = text
			}
		}
	}
	if runes := []rune(name); len(runes) > 255 {
		name = string(runes[:255])
	}
	return name
}

// pagesOf answers a document's pages, once XipFax has counted them.
func pagesOf(document xipfax.DraftDocument) string {
	count, _ := document.GetPageCountOk()
	switch {
	case count == nil:
		return ""
	case *count == 1:
		return ", 1 page"
	}
	return fmt.Sprintf(", %d pages", *count)
}
