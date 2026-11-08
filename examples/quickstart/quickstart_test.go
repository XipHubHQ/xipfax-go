package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/XipHubHQ/xipfax-go"
	"github.com/XipHubHQ/xipfax-go/internal/contract"
	"github.com/XipHubHQ/xipfax-go/internal/credentials"
	"github.com/XipHubHQ/xipfax-go/internal/standin"
)

// The quickstart, walked against a stand-in of the API that answers as the document says: its free
// steps first, its charged step said before it and taken only on send, and each request it sends
// read against the document, as each answer the stand-in gives is.

const (
	apiKey    = "xipfax_key_live_example"
	faxNumber = "+1 202 555 0143"
	pdfLink   = "https://example.com/letter.pdf"
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

// walk is what a walk saw: each request, line and question in its order, the requests themselves,
// and what the quickstart answered.
type walk struct {
	events    []string
	transport *standin.Transport
	err       error
}

// api is the API as the quickstart finds it, its draft holding documents to begin with, and
// addedOnXipfax too once the quickstart adds its own, as documents the client adds on xipfax.com
// meanwhile, each by its id.
type api struct {
	t             *testing.T
	document      *contract.Document
	walk          *walk
	documents     []string
	addedOnXipfax []string
	reads         int
}

// answer answers a request as the API would, and reads the answer against the document.
func (a *api) answer(request *http.Request, body []byte) (int, string) {
	route := request.Method + " " + request.URL.Path
	a.walk.events = append(a.walk.events, route)
	status, answer := a.route(route, body)
	if refused := a.document.ReadAnswer(request, status, []byte(answer)); len(refused) > 0 {
		a.t.Errorf("the stand-in's answer to %s is refused: %v", route, refused)
	}
	return status, answer
}

func (a *api) route(route string, body []byte) (int, string) {
	switch route {
	case "GET /quote":
		return http.StatusOK, standin.Quote
	case "GET /offices":
		return http.StatusOK, `{"data":[` + standin.Office + `],"has_more":false,"next_cursor":null}`
	case "GET /draft":
		documents := make([]string, len(a.documents))
		for index, id := range a.documents {
			documents[index] = standin.Document(id, "letter.pdf")
		}
		return http.StatusOK, `{"id":"fax_1","documents":[` + strings.Join(documents, ",") + `]}`
	case "POST /draft/documents":
		a.documents = append(append(a.documents, "doc_1"), a.addedOnXipfax...)
		return http.StatusCreated, `{"document":` + standin.Document("doc_1", "letter.pdf") + `,"upload":null}`
	case "DELETE /draft/documents/doc_1":
		a.documents = slices.DeleteFunc(a.documents, func(id string) bool { return id == "doc_1" })
		return http.StatusNoContent, ""
	case "POST /draft/send":
		// As the API does, it sends a draft only where the send names its documents, in order.
		var sent struct {
			DocumentIDs []string `json:"document_ids"`
		}
		if err := json.Unmarshal(body, &sent); err != nil || !slices.Equal(sent.DocumentIDs, a.documents) {
			return http.StatusConflict, standin.Refusal("draft_changed", "Your fax changed since this page showed it. Look it over, then send it again.")
		}
		return http.StatusOK, `{"fax":` + standin.Fax(map[string]any{"status": "sending", "step": "queued", "finished_at": nil}) + `,"paid_by":"plan","confirm_url":null}`
	case "GET /faxes/fax_1":
		a.reads++
		if a.reads < 2 {
			return http.StatusOK, standin.Fax(map[string]any{"status": "sending", "step": "sending", "finished_at": nil})
		}
		return http.StatusOK, standin.Fax(nil)
	case "GET /faxes/fax_1/confirmation":
		return http.StatusOK, `{"url":"https://example.com/confirmation.pdf"}`
	}
	return http.StatusNotFound, ""
}

// walkOf walks the quickstart against the API, its draft holding draft and gaining addedOnXipfax,
// typed being what you type when it asks, and reads each request it sent against the document.
func walkOf(t *testing.T, draft, addedOnXipfax []string, typed, to, pdfURL string) *walk {
	t.Helper()
	document, err := contract.Load("../../openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	seen := &walk{}
	stand := &api{t: t, document: document, walk: seen, documents: draft, addedOnXipfax: addedOnXipfax}
	seen.transport = &standin.Transport{Answer: stand.answer}
	client, err := xipfax.New(apiKey, xipfax.WithHTTPClient(seen.transport.Client()))
	if err != nil {
		t.Fatal(err)
	}

	seen.err = quickstart(context.Background(), client, input{
		to:     to,
		pdfURL: pdfURL,
		say:    func(line string) { seen.events = append(seen.events, "say: "+line) },
		ask: func(question string) string {
			seen.events = append(seen.events, "ask: "+question)
			return typed
		},
		pause: func(time.Duration) {},
	})

	for _, request := range seen.transport.Requests() {
		if _, refused := document.ReadRequest(request.Request, request.Body, xipfax.APIVersion); len(refused) > 0 {
			t.Errorf("%s %s: %v", request.Method, request.URL, refused)
		}
	}
	return seen
}

// requests answers the walk's requests alone, by their routes.
func (w *walk) requests() []string {
	var routes []string
	for _, event := range w.events {
		if !strings.HasPrefix(event, "say: ") && !strings.HasPrefix(event, "ask: ") {
			routes = append(routes, event)
		}
	}
	return routes
}

// sent answers the body of the walk's POST /draft/send.
func (w *walk) sent(t *testing.T) map[string]any {
	t.Helper()
	for _, request := range w.transport.Requests() {
		if request.Method == http.MethodPost && request.URL.Path == "/draft/send" {
			var body map[string]any
			if err := json.Unmarshal(request.Body, &body); err != nil {
				t.Fatal(err)
			}
			return body
		}
	}
	t.Fatal("the walk sent no POST /draft/send")
	return nil
}

func asks(event string) bool { return strings.HasPrefix(event, "ask: ") }

func TestTheQuickstartTakesTheFreeStepsAloneWhereItHasNoNumberOrPDF(t *testing.T) {
	seen := walkOf(t, nil, nil, "send", "", "")

	if seen.err != nil {
		t.Fatal(seen.err)
	}
	if got, want := seen.requests(), []string{"GET /quote", "GET /offices"}; !reflect.DeepEqual(got, want) {
		t.Errorf("it sent %v, not %v", got, want)
	}
	if slices.ContainsFunc(seen.events, asks) {
		t.Errorf("it asked, with no number or PDF: %v", seen.events)
	}
}

func TestTheQuickstartSaysTheFaxIsRealAndChargedSendsItOnlyOnceSendIsTypedAndFollowsIt(t *testing.T) {
	seen := walkOf(t, nil, nil, "send", faxNumber, pdfLink)

	if seen.err != nil {
		t.Fatal(seen.err)
	}
	want := []string{
		"GET /quote",
		"GET /offices",
		"GET /draft",
		"POST /draft/documents",
		"POST /draft/send",
		"GET /faxes/fax_1",
		"GET /faxes/fax_1",
		"GET /faxes/fax_1/confirmation",
	}
	if got := seen.requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("it sent %v, not %v", got, want)
	}
	added := slices.Index(seen.events, "POST /draft/documents")
	warned := slices.Index(seen.events, "say: "+realAndCharged)
	asked := slices.IndexFunc(seen.events, asks)
	sent := slices.Index(seen.events, "POST /draft/send")
	if added < 0 || warned < added || asked < warned || sent < asked {
		t.Errorf("it added at %d, warned at %d, asked at %d and sent at %d: %v", added, warned, asked, sent, seen.events)
	}
	if !slices.Contains(seen.events, "say: Added letter.pdf, 2 pages.") {
		t.Errorf("it didn't say what it added: %v", seen.events)
	}

	if got, want := seen.sent(t), map[string]any{"fax_number": faxNumber, "document_ids": []any{"doc_1"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("the send was %v, not %v", got, want)
	}
	keys := map[string]bool{}
	posts := 0
	for _, request := range seen.transport.Requests() {
		if request.Method == http.MethodPost {
			keys[request.Header.Get("Idempotency-Key")] = true
			posts++
		}
	}
	if len(keys) != posts {
		t.Errorf("%d POSTs named %d keys: %v", posts, len(keys), keys)
	}
}

func TestTheQuickstartSendsNothingOnAnyOtherAnswerAndTakesItsDocumentBackOutOfTheDraft(t *testing.T) {
	seen := walkOf(t, nil, nil, "no", faxNumber, pdfLink)

	if seen.err != nil {
		t.Fatal(seen.err)
	}
	want := []string{"GET /quote", "GET /offices", "GET /draft", "POST /draft/documents", "DELETE /draft/documents/doc_1"}
	if got := seen.requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("it sent %v, not %v", got, want)
	}
}

func TestTheQuickstartNamesOnlyTheDocumentItAddedSoADraftThatGainedAnotherOnXipFaxIsRefused(t *testing.T) {
	seen := walkOf(t, nil, []string{"doc_2"}, "send", faxNumber, pdfLink)

	var refused *xipfax.GenericOpenAPIError
	if !errors.As(seen.err, &refused) {
		t.Fatalf("it answered %v, not the API's refusal", seen.err)
	}
	if refusal, ok := refused.Model().(xipfax.APIError); !ok || refusal.Error.Code != "draft_changed" {
		t.Errorf("the refusal is %+v", refused.Model())
	}
	want := []string{"GET /quote", "GET /offices", "GET /draft", "POST /draft/documents", "POST /draft/send"}
	if got := seen.requests(); !reflect.DeepEqual(got, want) {
		t.Errorf("it sent %v, not %v", got, want)
	}
	if got, want := seen.sent(t), map[string]any{"fax_number": faxNumber, "document_ids": []any{"doc_1"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("the send was %v, not %v", got, want)
	}
}

func TestTheQuickstartAddsNothingToADraftThatAlreadyHoldsDocuments(t *testing.T) {
	seen := walkOf(t, []string{"doc_0"}, nil, "send", faxNumber, pdfLink)

	if seen.err != nil {
		t.Fatal(seen.err)
	}
	if got, want := seen.requests(), []string{"GET /quote", "GET /offices", "GET /draft"}; !reflect.DeepEqual(got, want) {
		t.Errorf("it sent %v, not %v", got, want)
	}
}

func TestTheDocumentIsNamedAsItsLinkEnds(t *testing.T) {
	long := strings.Repeat("é", 300)
	for link, want := range map[string]string{
		"https://example.com/letter.pdf":            "letter.pdf",
		"https://example.com/forms/Form%202848.pdf": "Form 2848.pdf",
		"https://example.com/forms/":                "forms",
		"https://example.com":                       "document.pdf",
		"https://example.com/%FF.pdf":               "%FF.pdf",
		"https://example.com/" + long:               strings.Repeat("é", 255),
	} {
		if got := nameOf(link); got != want {
			t.Errorf("nameOf(%q) is %q, not %q", link, got, want)
		}
	}
}
