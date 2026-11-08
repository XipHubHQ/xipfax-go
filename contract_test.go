package xipfax_test

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/XipHubHQ/xipfax-go"
	"github.com/XipHubHQ/xipfax-go/internal/contract"
	"github.com/XipHubHQ/xipfax-go/internal/standin"
)

// Each of the SDK's methods sends the request the API's document asks for: its method and path on
// the document's server, the key as a bearer token, the version, and every parameter and body as
// the document's schemas take them. The calls are typed by the SDK, so they compile only as the
// generated code allows, and each request they send is read against the document itself, as each
// answer the stand-in gives is. Each event the document posts is typed by the SDK as FaxEvent,
// which the document takes as the SDK writes it.

// documentOf reads the API's document, as the repository holds it.
func documentOf(t *testing.T) *contract.Document {
	t.Helper()
	document, err := contract.Load("openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	return document
}

// calls are one call of each operation, by its operation id, with arguments the document takes.
var calls = map[string]func(context.Context, *xipfax.APIClient) error{
	"getQuote": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.QuoteAPI.GetQuote(ctx).Pages(3).Execute()
		return err
	},
	"getDraft": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.DraftAPI.GetDraft(ctx).Execute()
		return err
	},
	"addDocument": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.DraftAPI.AddDocument(ctx).
			IdempotencyKey("add-letter-1").
			NewDocument(xipfax.NewDocument{Name: "letter.pdf", URL: xipfax.PtrString("https://example.com/letter.pdf")}).
			Execute()
		return err
	},
	"completeDocument": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.DraftAPI.CompleteDocument(ctx, "doc_1").IdempotencyKey("complete-letter-1").Execute()
		return err
	},
	"removeDocument": func(ctx context.Context, client *xipfax.APIClient) error {
		_, err := client.DraftAPI.RemoveDocument(ctx, "doc_1").Execute()
		return err
	},
	"sendDraft": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.DraftAPI.SendDraft(ctx).
			IdempotencyKey("send-letter-1").
			SendDraft(xipfax.SendDraft{FaxNumber: xipfax.PtrString("+1 202 555 0143"), DocumentIDs: []string{"doc_1"}}).
			Execute()
		return err
	},
	"listFaxes": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.FaxesAPI.ListFaxes(ctx).Limit(20).StartingAfter("fax_0").Direction("sent").Execute()
		return err
	},
	"getFax": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.FaxesAPI.GetFax(ctx, "fax_1").Execute()
		return err
	},
	"getConfirmation": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.FaxesAPI.GetConfirmation(ctx, "fax_1").Execute()
		return err
	},
	"getPlan": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.PlanAPI.GetPlan(ctx).Execute()
		return err
	},
	"listOffices": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.OfficesAPI.ListOffices(ctx).Search("Form 2848").Place("OH").Limit(5).StartingAfter("office_0").Execute()
		return err
	},
	"registerWebhookEndpoint": func(ctx context.Context, client *xipfax.APIClient) error {
		_, _, err := client.WebhookEndpointsAPI.RegisterWebhookEndpoint(ctx).
			IdempotencyKey("register-endpoint-1").
			WebhookEndpointRegistration(xipfax.WebhookEndpointRegistration{URL: "https://example.com/xipfax/events"}).
			Execute()
		return err
	},
	"removeWebhookEndpoint": func(ctx context.Context, client *xipfax.APIClient) error {
		_, err := client.WebhookEndpointsAPI.RemoveWebhookEndpoint(ctx, "endpoint_1").Execute()
		return err
	},
}

// answers are what the stand-in answers each operation's success with, by its operation id: none
// for the two that answer 204.
var answers = map[string]string{
	"getQuote":                standin.Quote,
	"getDraft":                `{"id":"fax_1","documents":[]}`,
	"addDocument":             `{"document":` + standin.Document("doc_1", "letter.pdf") + `,"upload":null}`,
	"completeDocument":        standin.Document("doc_1", "letter.pdf"),
	"sendDraft":               `{"fax":` + standin.Fax(map[string]any{"status": "sending", "step": "queued", "finished_at": nil}) + `,"paid_by":"plan","confirm_url":null}`,
	"listFaxes":               `{"data":[` + standin.Fax(nil) + `],"has_more":false,"next_cursor":null}`,
	"getFax":                  standin.Fax(nil),
	"getConfirmation":         `{"url":"https://example.com/confirmation.pdf"}`,
	"getPlan":                 standin.NoPlan,
	"listOffices":             `{"data":[` + standin.Office + `],"has_more":false,"next_cursor":null}`,
	"registerWebhookEndpoint": `{"id":"endpoint_1","url":"https://example.com/xipfax/events","created_at":"2026-11-07T02:00:00Z","secret":"whsec_c3RhbmQtaW4="}`,
}

func TestTheSDKHasACallForEveryOperationTheDocumentNamesAndNoOther(t *testing.T) {
	var ids []string
	for _, operation := range documentOf(t).Operations {
		ids = append(ids, operation.ID)
	}
	var named []string
	for id := range calls {
		named = append(named, id)
	}
	sort.Strings(ids)
	sort.Strings(named)
	if !reflect.DeepEqual(named, ids) {
		t.Errorf("the calls are %v, and the document's operations %v", named, ids)
	}
}

func TestTheSDKNamesTheVersionTheDocumentIsTheOneXipVersionTakes(t *testing.T) {
	document := documentOf(t)
	if xipfax.APIVersion != document.Version {
		t.Errorf("APIVersion is %s, and the document %s", xipfax.APIVersion, document.Version)
	}
	for _, operation := range document.Operations {
		if refused := document.VersionRefused(operation, xipfax.APIVersion); len(refused) > 0 {
			t.Errorf("%s: %v", operation.ID, refused)
		}
	}
}

func TestEachCallSendsItsRequestAsTheDocumentAsks(t *testing.T) {
	document := documentOf(t)
	for _, operation := range document.Operations {
		t.Run(operation.ID, func(t *testing.T) {
			call := calls[operation.ID]
			if call == nil {
				t.Fatalf("no call of %s", operation.ID)
			}
			answer := answers[operation.ID]
			transport := &standin.Transport{Answer: standin.Same(operation.Success, answer)}

			if err := call(context.Background(), clientOf(t, "xipfax_key_live_example", transport)); err != nil {
				t.Fatalf("%s answered %v", operation.ID, err)
			}

			requests := transport.Requests()
			if len(requests) != 1 {
				t.Fatalf("%s sent %d requests", operation.ID, len(requests))
			}
			request := requests[0]
			if got := request.Header.Values("Authorization"); !reflect.DeepEqual(got, []string{"Bearer xipfax_key_live_example"}) {
				t.Errorf("Authorization is %q", got)
			}
			id, refused := document.ReadRequest(request.Request, request.Body, xipfax.APIVersion)
			if id != operation.ID || len(refused) > 0 {
				t.Errorf("%s %s reads as %q, refused: %v", request.Method, request.URL, id, refused)
			}
			if refused := document.ReadAnswer(request.Request, operation.Success, []byte(answer)); len(refused) > 0 {
				t.Errorf("the stand-in's answer is refused: %v", refused)
			}
		})
	}
}

func TestEachEventTheDocumentPostsIsAFaxEventItTakes(t *testing.T) {
	document := documentOf(t)
	// The fax each event tells of, by the event's type: one for each type the document posts.
	faxes := map[string]string{
		"fax.delivered": standin.Fax(map[string]any{"price": map[string]any{"amount_minor": 100, "currency": "USD"}}),
		"fax.failed":    standin.Fax(map[string]any{"status": "failed", "failure": "busy"}),
		"fax.received": standin.Fax(map[string]any{
			"id": "fax_2", "direction": "received", "status": "received", "from_number": "+12025550111",
			"to_number": "+12025550100", "pages": 3, "finished_at": nil, "time_zone": nil,
		}),
	}
	var posted, typed []string
	for name := range document.Webhooks {
		posted = append(posted, name)
	}
	for name := range faxes {
		typed = append(typed, name)
	}
	sort.Strings(posted)
	sort.Strings(typed)
	if !reflect.DeepEqual(posted, typed) {
		t.Fatalf("the document posts %v, and the test types %v", posted, typed)
	}

	for name, fax := range faxes {
		var data xipfax.Fax
		if err := json.Unmarshal([]byte(fax), &data); err != nil {
			t.Fatalf("%s: the SDK doesn't read its fax: %v", name, err)
		}
		event := xipfax.FaxEvent{ID: "event_1", Type: name, APIVersion: xipfax.APIVersion, CreatedAt: "2026-11-07T02:03:00Z", Data: data}
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		var value any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		if refused := document.Check(document.Webhooks[name], value, name); len(refused) > 0 {
			t.Errorf("%s as the SDK writes it is refused: %v", name, refused)
		}
		var read xipfax.FaxEvent
		if err := json.Unmarshal(body, &read); err != nil || read.Data.ID != data.ID {
			t.Errorf("%s: the SDK doesn't read back the event it wrote: %v", name, err)
		}
	}
}

// The reader is held to failing too: each thing the document refuses in a request or an answer is
// refused, by name, so that a test reading the SDK's requests as fitting can fail.

// requestOf answers a request of method to address, with headers and body as they are given.
func requestOf(t *testing.T, method, address string, headers http.Header, body string) (*http.Request, []byte) {
	t.Helper()
	request, err := http.NewRequest(method, address, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header = headers
	return request, []byte(body)
}

func TestTheReaderRefusesEachThingARequestGetsWrong(t *testing.T) {
	document := documentOf(t)
	signed := func(more http.Header) http.Header {
		headers := http.Header{"Authorization": {"Bearer xipfax_key_live_example"}, "Xip-Version": {xipfax.APIVersion}}
		for name, values := range more {
			headers[name] = values
		}
		return headers
	}
	for _, c := range []struct {
		name, method, address string
		headers               http.Header
		body                  string
		id                    string
		refused               []string
	}{
		{
			name: "a request it takes", method: "GET", address: "https://api.xipfax.com/quote?pages=3",
			headers: signed(nil), id: "getQuote",
		},
		{
			name:   "no key, the version twice, a page past the most and a parameter it doesn't take",
			method: "GET", address: "https://api.xipfax.com/quote?pages=251&color=red",
			headers: http.Header{"Xip-Version": {xipfax.APIVersion, xipfax.APIVersion}}, id: "getQuote",
			refused: []string{
				"Authorization is [], not one bearer key",
				`Xip-Version is ["2026-11-03" "2026-11-03"], not 2026-11-03`,
				"header Xip-Version is sent 2 times",
				"query pages is 251, past its maximum of 250",
				"query color is no parameter of getQuote",
			},
		},
		{
			name:   "a body with a member it doesn't take, a size under the least and no name",
			method: "POST", address: "https://api.xipfax.com/draft/documents",
			headers: signed(http.Header{"Content-Type": {"application/json"}, "Idempotency-Key": {"add-letter-1"}}),
			body:    `{"size_bytes":0,"colour":"red"}`, id: "addDocument",
			refused: []string{
				"body.colour is no member it takes",
				"body.size_bytes is 0, past its minimum of 1",
				"body.name is missing",
			},
		},
		{
			name: "a body where the operation takes none", method: "DELETE", address: "https://api.xipfax.com/draft/documents/doc_1",
			headers: signed(nil), body: "{}", id: "removeDocument",
			refused: []string{"a body the operation takes none of"},
		},
		{
			name: "another server", method: "GET", address: "https://api.example.com/quote?pages=3", headers: signed(nil),
			refused: []string{"GET https://api.example.com/quote?pages=3 is no operation of the document"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			request, body := requestOf(t, c.method, c.address, c.headers, c.body)
			id, refused := document.ReadRequest(request, body, xipfax.APIVersion)
			if id != c.id || !reflect.DeepEqual(refused, c.refused) {
				t.Errorf("read as %q, refused %q; not %q, refused %q", id, refused, c.id, c.refused)
			}
		})
	}
}

func TestTheReaderRefusesEachThingAnAnswerGetsWrong(t *testing.T) {
	document := documentOf(t)
	for _, c := range []struct {
		name, method, address string
		status                int
		answer                string
		refused               []string
	}{
		{
			name:   "a fax whose pages are words and whose status the document doesn't list",
			method: "GET", address: "https://api.xipfax.com/faxes/fax_1", status: http.StatusOK,
			answer: standin.Fax(map[string]any{"pages": "two", "status": "archived"}),
			refused: []string{
				"answer.pages is string, not [integer null]",
				"answer.status is archived, none of [draft sending delivered failed received]",
			},
		},
		{
			name: "a status the operation doesn't give", method: "GET", address: "https://api.xipfax.com/plan",
			status: http.StatusTeapot, refused: []string{"getPlan gives no 418"},
		},
		{
			name: "a body where the answer carries none", method: "DELETE", address: "https://api.xipfax.com/draft/documents/doc_1",
			status: http.StatusNoContent, answer: "{}", refused: []string{"removeDocument's 204 carries no body"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			request, _ := requestOf(t, c.method, c.address, http.Header{}, "")
			if refused := document.ReadAnswer(request, c.status, []byte(c.answer)); !reflect.DeepEqual(refused, c.refused) {
				t.Errorf("refused %q, not %q", refused, c.refused)
			}
		})
	}
}

func TestTheReaderRefusesASchemaKeywordItDoesntRead(t *testing.T) {
	schema := map[string]any{"oneOf": []any{map[string]any{"type": "string"}}}
	want := []string{"value: the contract reads no oneOf, which its schema gives"}
	if refused := documentOf(t).Check(schema, "text", "value"); !reflect.DeepEqual(refused, want) {
		t.Errorf("refused %q, not %q", refused, want)
	}
}
