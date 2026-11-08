// Package standin answers the SDK's requests in place of the API, in the tests: an http.RoundTripper
// that keeps each request it is given, with its body, and answers it as its Answer says. Nothing it
// does reaches a network.
package standin

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// A Request is one the stand-in was given, its body as it was sent.
type Request struct {
	*http.Request
	Body []byte
}

// An Answer gives the status and the JSON body the stand-in answers a request with; an empty body
// answers none.
type Answer func(request *http.Request, body []byte) (status int, answer string)

// Transport is the stand-in, an http.RoundTripper for an http.Client's Transport.
type Transport struct {
	Answer Answer

	mu       sync.Mutex
	requests []Request
}

// Client answers an http.Client whose every request goes to the stand-in.
func (t *Transport) Client() *http.Client {
	return &http.Client{Transport: t}
}

// Requests answers each request the stand-in was given, in its order.
func (t *Transport) Requests() []Request {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Request(nil), t.requests...)
}

// RoundTrip keeps the request and answers it as Answer says.
func (t *Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	var body []byte
	if request.Body != nil {
		read, err := io.ReadAll(request.Body)
		request.Body.Close()
		if err != nil {
			return nil, err
		}
		body = read
	}
	t.mu.Lock()
	t.requests = append(t.requests, Request{Request: request, Body: body})
	t.mu.Unlock()

	status, answer := t.Answer(request, body)
	header := http.Header{}
	if answer != "" {
		header.Set("Content-Type", "application/json")
	}
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(strings.NewReader(answer)),
		ContentLength: int64(len(answer)),
		Request:       request,
	}, nil
}

// Same answers every request with one status and one body.
func Same(status int, answer string) Answer {
	return func(*http.Request, []byte) (int, string) {
		return status, answer
	}
}
