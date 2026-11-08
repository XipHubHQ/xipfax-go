package xipfax_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/XipHubHQ/xipfax-go"
	"github.com/XipHubHQ/xipfax-go/internal/standin"
)

// clientOf answers a client of key whose every request goes to the stand-in.
func clientOf(t *testing.T, key string, transport *standin.Transport, options ...xipfax.Option) *xipfax.APIClient {
	t.Helper()
	client, err := xipfax.New(key, append(options, xipfax.WithHTTPClient(transport.Client()))...)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestNewRefusesToStartWithoutAKey(t *testing.T) {
	for _, apiKey := range []string{"", "   "} {
		client, err := xipfax.New(apiKey)
		if !errors.Is(err, xipfax.ErrNoAPIKey) || client != nil {
			t.Errorf("New(%q) answered %v and %v, not ErrNoAPIKey", apiKey, client, err)
		}
	}
}

func TestNewSendsToTheAPIsOwnAddressUnlessGivenAnother(t *testing.T) {
	own := &standin.Transport{Answer: standin.Same(http.StatusOK, standin.NoPlan)}
	if _, _, err := clientOf(t, "key-a", own).PlanAPI.GetPlan(context.Background()).Execute(); err != nil {
		t.Fatal(err)
	}
	if got := own.Requests()[0].URL.String(); got != "https://api.xipfax.com/plan" {
		t.Errorf("GET /plan went to %s", got)
	}

	other := &standin.Transport{Answer: standin.Same(http.StatusOK, standin.NoPlan)}
	client := clientOf(t, "key-a", other, xipfax.WithBaseURL("https://api.example.com/"))
	if _, _, err := client.PlanAPI.GetPlan(context.Background()).Execute(); err != nil {
		t.Fatal(err)
	}
	if got := other.Requests()[0].URL.String(); got != "https://api.example.com/plan" {
		t.Errorf("GET /plan went to %s, not the address named", got)
	}
}

func TestEachClientKeepsItsKeyToItself(t *testing.T) {
	a := &standin.Transport{Answer: standin.Same(http.StatusOK, standin.NoPlan)}
	b := &standin.Transport{Answer: standin.Same(http.StatusOK, standin.NoPlan)}
	first := clientOf(t, "key-a", a)
	second := clientOf(t, "key-b", b)

	for _, client := range []*xipfax.APIClient{first, second} {
		if _, _, err := client.PlanAPI.GetPlan(context.Background()).Execute(); err != nil {
			t.Fatal(err)
		}
	}

	for transport, want := range map[*standin.Transport]string{a: "Bearer key-a", b: "Bearer key-b"} {
		requests := transport.Requests()
		if len(requests) != 1 {
			t.Fatalf("%d requests went where one should have", len(requests))
		}
		if got := requests[0].Header.Values("Authorization"); !reflect.DeepEqual(got, []string{want}) {
			t.Errorf("Authorization is %q, not %q", got, want)
		}
	}
}

func TestEachRequestNamesTheVersionTheSDKIsGeneratedFrom(t *testing.T) {
	transport := &standin.Transport{Answer: standin.Same(http.StatusOK, standin.NoPlan)}
	if _, _, err := clientOf(t, "key-a", transport).PlanAPI.GetPlan(context.Background()).Execute(); err != nil {
		t.Fatal(err)
	}
	if got := transport.Requests()[0].Header.Values("Xip-Version"); !reflect.DeepEqual(got, []string{xipfax.APIVersion}) {
		t.Errorf("Xip-Version is %q, not %q", got, xipfax.APIVersion)
	}
}

func TestARefusalIsTheAPIsOwnError(t *testing.T) {
	refusal := standin.Refusal("authentication_failed", "That key isn’t one of XipFax’s.")
	transport := &standin.Transport{Answer: standin.Same(http.StatusUnauthorized, refusal)}

	answer, response, err := clientOf(t, "key-a", transport).PlanAPI.GetPlan(context.Background()).Execute()

	if answer != nil {
		t.Errorf("a refusal answered %v", answer)
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Errorf("the response is %v, not the 401", response)
	}
	var refused *xipfax.GenericOpenAPIError
	if !errors.As(err, &refused) {
		t.Fatalf("the error is %T, %v, not a *GenericOpenAPIError", err, err)
	}
	apiErr, ok := refused.Model().(xipfax.APIError)
	if !ok {
		t.Fatalf("its model is %T, not an APIError", refused.Model())
	}
	if apiErr.Error.Code != "authentication_failed" || apiErr.Error.RequestID != "req_1" {
		t.Errorf("the refusal reads %+v", apiErr.Error)
	}
}

func TestAnAnswerThatGainsAMemberIsReadAndCarriesItThrough(t *testing.T) {
	// The document's Additions rule: within a version, an answer may gain a member, and an open set
	// a new value. Here the fax gains `tracking`, and its status is one the SDK has never seen.
	gained := standin.Fax(map[string]any{"status": "archived", "tracking": map[string]any{"carrier": "stand-in"}})
	transport := &standin.Transport{Answer: standin.Same(http.StatusOK, gained)}

	fax, _, err := clientOf(t, "key-a", transport).FaxesAPI.GetFax(context.Background(), "fax_1").Execute()

	if err != nil {
		t.Fatalf("an answer that gained a member was refused: %v", err)
	}
	if fax.Status != "archived" {
		t.Errorf("the new status reads %q", fax.Status)
	}
	again, err := json.Marshal(fax)
	if err != nil {
		t.Fatal(err)
	}
	var carried map[string]any
	if err := json.Unmarshal(again, &carried); err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"carrier": "stand-in"}; !reflect.DeepEqual(carried["tracking"], want) {
		t.Errorf("the gained member isn't carried through: %s", again)
	}
}
