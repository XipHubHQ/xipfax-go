package standin

import "encoding/json"

// The API's answers the tests are given, in JSON, each as the document gives it.

// Quote is GET /quote's answer for a fax of one page.
const Quote = `{"pages":1,"price":{"amount_minor":100,"currency":"USD"},"price_shown":"$1.00","terms":"The stand-in’s terms.","price_list":"stand-in"}`

// Office is an office of GET /offices, which takes Form 2848 from Ohio.
const Office = `{"id":"stand-in-office","name":"A stand-in office","fax_number":"+12025550143","fax_number_from_abroad":null,"applies_to":["Ohio"],"takes":["Form 2848"],"signed_in_ink":false,"confidential_cover":false,"sources":[{"title":"A stand-in page","url":"https://example.com/office"}],"checked_on":"2026-11-01"}`

// NoPlan is GET /plan's answer for a client with no plan.
const NoPlan = `{"plan":null}`

// Document answers a document of the draft, ready, by its id and its name.
func Document(id, name string) string {
	return object(map[string]any{
		"id":         id,
		"name":       name,
		"size_bytes": 20480,
		"page_count": 2,
		"status":     "ready",
		"url":        "https://example.com/" + id + ".pdf",
	})
}

// Fax answers fax_1, a fax you sent that was delivered, with each member changes names in place of
// its own.
func Fax(changes map[string]any) string {
	fax := map[string]any{
		"id":                 "fax_1",
		"direction":          "sent",
		"status":             "delivered",
		"step":               nil,
		"office_id":          nil,
		"to_number":          "+12025550143",
		"from_number":        "+12025550100",
		"pages":              2,
		"price":              nil,
		"sent_at":            "2026-11-07T02:00:00Z",
		"finished_at":        "2026-11-07T02:03:00Z",
		"retry_at":           nil,
		"failure":            nil,
		"time_zone":          "America/New_York",
		"sender_name":        nil,
		"cover_confidential": false,
	}
	for name, value := range changes {
		fax[name] = value
	}
	return object(fax)
}

// Refusal answers the API's refusal by its code and its sentence.
func Refusal(code, message string) string {
	return object(map[string]any{"error": map[string]any{
		"code":       code,
		"message":    message,
		"details":    []any{},
		"request_id": "req_1",
		"doc_url":    nil,
	}})
}

// object answers members as a JSON object, its members in the order of their names.
func object(members map[string]any) string {
	data, err := json.Marshal(members)
	if err != nil {
		panic("standin: an answer of plain values doesn't marshal: " + err.Error())
	}
	return string(data)
}
