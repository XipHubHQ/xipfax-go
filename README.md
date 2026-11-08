# XipFax for Go

Send faxes from your code with [XipFax](https://xipfax.com): quote a fax, add your PDFs, send them to a
number or a government office, and follow each fax until it's delivered. The SDK is generated from
the API's own document, so its methods and types are the API's. It runs on Go 1.23 or later, and
needs nothing beyond the standard library.

## Install

```sh
go get github.com/XipHubHQ/xipfax-go
```

## Make an API key

Sign in to the [developer console](https://developer.xipfax.com/console/), open API keys and make a
key. You see it whole just once, so copy it then. Every key is live: what it sends is real.

Keep the key on your server. A key in a web page or an app is anyone's.

## Use it

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/XipHubHQ/xipfax-go"
)

func main() {
	client, err := xipfax.New(os.Getenv("XIPFAX_API_KEY"))
	if err != nil {
		log.Fatal(err)
	}

	quote, _, err := client.QuoteAPI.GetQuote(context.Background()).Pages(3).Execute()
	var refused *xipfax.GenericOpenAPIError
	if errors.As(err, &refused) {
		if refusal, ok := refused.Model().(xipfax.APIError); ok {
			log.Fatalf("%s: %s", refusal.Error.Code, refusal.Error.Message)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %s\n", quote.PriceShown, quote.Terms)
}
```

The client's services are the [API reference](https://developer.xipfax.com/docs/api/)'s sections:
`QuoteAPI`, `DraftAPI`, `FaxesAPI`, `PlanAPI`, `OfficesAPI` and `WebhookEndpointsAPI`. Each method
starts a request, which takes what it carries by name, `.Pages(3)` or `.IdempotencyKey(key)`, a path's
parameters as the method's arguments; `Execute` sends it. It answers what the API gave, the
`*http.Response`, and an error: where the API refused, a `*xipfax.GenericOpenAPIError` whose `Model`
is the API's error object, an `APIError`, whose `Code` is the one to branch on. A request that never
reached XipFax answers the error `net/http` gave.

`xipfax.New` takes options: `xipfax.WithHTTPClient` for the `*http.Client` each request is sent with,
and `xipfax.WithBaseURL` for another address than `https://api.xipfax.com`.

## The quickstart

[`examples/quickstart`](examples/quickstart) takes the steps of
[XipFax's quickstart](https://developer.xipfax.com/docs/guides/quickstart/) through the SDK. Run it
from a clone of this repository:

```sh
XIPFAX_API_KEY=xipfax_key_live_… go run ./examples/quickstart
```

That takes the free steps alone: what a fax costs, and the offices that take Form 2848 from Ohio,
with their fax numbers.

To send a fax too, name a fax number of yours and a public https link to a PDF:

```sh
XIPFAX_TO='+1 202 555 0143' XIPFAX_PDF_URL=https://example.com/letter.pdf \
  XIPFAX_API_KEY=xipfax_key_live_… go run ./examples/quickstart
```

**That step sends a real fax, and it's charged.** Its pages count on your plan, or where you have no
plan its price is held on the card you saved, and taken once it's delivered. A fax that fails costs
nothing. The quickstart says so before it sends, and sends only once you type `send`. Then it
follows the fax until it's delivered, and gives the link to its confirmation.

## Methods

| Method                                        | Request                                        | What it does                                                                         |
| --------------------------------------------- | ---------------------------------------------- | ------------------------------------------------------------------------------------ |
| `QuoteAPI.GetQuote`                           | `GET /quote`                                   | The price of a fax of so many pages, sent without a plan.                            |
| `DraftAPI.GetDraft`                           | `GET /draft`                                   | Your draft, the fax you prepare, its documents in the order it sends them.           |
| `DraftAPI.AddDocument`                        | `POST /draft/documents`                        | Adds a PDF to the draft, by a link XipFax fetches or by an upload.                   |
| `DraftAPI.CompleteDocument`                   | `POST /draft/documents/{document_id}/complete` | Reads an uploaded PDF back, its pages counted.                                       |
| `DraftAPI.RemoveDocument`                     | `DELETE /draft/documents/{document_id}`        | Takes a document out of the draft.                                                   |
| `DraftAPI.SendDraft`                          | `POST /draft/send`                             | Sends the draft to a number or an office.                                            |
| `FaxesAPI.ListFaxes`                          | `GET /faxes`                                   | The faxes you sent and received, newest first, a page at a time.                     |
| `FaxesAPI.GetFax`                             | `GET /faxes/{fax_id}`                          | A fax, where it stands now.                                                          |
| `FaxesAPI.GetConfirmation`                    | `GET /faxes/{fax_id}/confirmation`             | A delivered fax's confirmation, by a link that reads it for a few minutes.           |
| `PlanAPI.GetPlan`                             | `GET /plan`                                    | Your plan, or none.                                                                  |
| `OfficesAPI.ListOffices`                      | `GET /offices`                                 | The government offices XipFax gives the fax numbers of.                              |
| `WebhookEndpointsAPI.RegisterWebhookEndpoint` | `POST /webhook-endpoints`                      | Registers an endpoint of your server for every event, its signing secret shown once. |
| `WebhookEndpointsAPI.RemoveWebhookEndpoint`   | `DELETE /webhook-endpoints/{endpoint_id}`      | Takes an endpoint away: no event goes to it from then on.                            |

### Upload a PDF from your disk

Give its size, send its bytes to the link the answer gives, then complete it:

```go
pdf, err := os.ReadFile("letter.pdf")
if err != nil {
	return err
}
added, _, err := client.DraftAPI.AddDocument(ctx).
	IdempotencyKey(addKey).
	NewDocument(xipfax.NewDocument{Name: "letter.pdf", SizeBytes: xipfax.PtrInt32(int32(len(pdf)))}).
	Execute()
if err != nil {
	return err
}
upload := added.Upload.Get()
if upload == nil {
	return errors.New("no link to upload to: the document is ready or removed")
}
put, err := http.NewRequestWithContext(ctx, http.MethodPut, upload.URL, bytes.NewReader(pdf))
if err != nil {
	return err
}
for name, value := range upload.Headers {
	put.Header.Set(name, value)
}
response, err := http.DefaultClient.Do(put)
if err != nil {
	return err
}
response.Body.Close()
if response.StatusCode >= 300 {
	return fmt.Errorf("the upload failed: %s", response.Status)
}
_, _, err = client.DraftAPI.CompleteDocument(ctx, added.Document.ID).IdempotencyKey(completeKey).Execute()
return err
```

`addKey` and `completeKey` are keys of your own, one for each request, as the next section says.

## Retries and `Idempotency-Key`

`AddDocument`, `CompleteDocument`, `SendDraft` and `RegisterWebhookEndpoint` take an
`Idempotency-Key` of your own, such as a UUID, which you keep with the request. Sent again with the
same key within 24 hours, the request is answered as it was the first time rather than done twice,
so a retry after a timeout can't send a fax twice. The SDK makes up no key itself: one it made would
be new on each try, and protect nothing.

The answer to a request sent again keeps none of its signed links, nor an endpoint's secret: a
document's `URL` and an endpoint's `Secret` come back null. `Upload` is signed again instead while
its document is still uploading: asked for again with its key, `AddDocument` gives a fresh link to
send the bytes to, and null once the document is ready or removed. An endpoint whose first answer
you lost has no secret to check its deliveries by: remove it with `RemoveWebhookEndpoint`, and
register it again with a new key.

## Webhooks

`RegisterWebhookEndpoint` registers an endpoint of your server, which every event goes to from then
on: when a fax you sent is delivered or fails for good, and when a fax sent to your number arrives.
Its answer shows the endpoint's signing secret once, so keep it then. `RemoveWebhookEndpoint` takes
an endpoint away, as revoking the key that registered it does. Both take an API key, not an app's
token.

Check each delivery's `webhook-signature` with that secret, against its body exactly as it came,
before you trust it. [XipFax's webhooks guide](https://developer.xipfax.com/docs/guides/webhooks/)
shows how. Its body is then a `xipfax.FaxEvent`, which `json.Unmarshal` reads.

## Versions

Each request names the version of the API the SDK was generated from, `xipfax.APIVersion`, in its
`Xip-Version` header, so each answer keeps the shape the SDK's types describe. Within that version,
an answer may gain a member, and a set of values a new value: the SDK reads both, and keeps a member
it doesn't know in the model's `AdditionalProperties`.

## Where the SDK comes from

`openapi.json` is the API's document, as [api.xipfax.com](https://api.xipfax.com/openapi.json) gives
it, and `scripts/generate.sh` makes the SDK from it with
[openapi-generator](https://openapi-generator.tech) 7.17.0's Go generator and
`openapi-generator.yaml`. It first writes the document the SDK is generated from, by
`scripts/sdk-document.jq`, which sets aside what the SDK sends itself or doesn't offer: the
document's security, since `New` sends your key on every request, and an app's token is not the
SDK's; the per-request `Xip-Version`, since `New` names the version on every request; and the
webhooks, which are XipFax's requests to your server, their bodies typed as `FaxEvent`. Nothing the
generator writes is edited by hand: `xipfax.go` and `doc.go` are the hand-written part. To update
the SDK, copy the new document over `openapi.json`, run `scripts/generate.sh`, and commit what
changes. `scripts/gates.sh` checks that the SDK is what the document generates, formatted, vetted
and built; `scripts/test.sh` runs the tests.

## Help

Write to [help@xipfax.com](mailto:help@xipfax.com).

## License

[MIT](LICENSE).
