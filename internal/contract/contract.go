// Package contract reads what the SDK sends and is answered against the API's document,
// openapi.json, as the repository holds it: the operation a request is, and each thing in it the
// document refuses, from its address, its key and its version to each parameter and its body; and
// what in an answer or an event's body the document's schema refuses. It reads the schema keywords
// the document uses, and refuses a schema with one it doesn't read, so that a document that comes
// to say more than this reads fails the tests rather than passing unread.
package contract

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Document is the API's document.
type Document struct {
	raw    map[string]any
	server *url.URL
	// Version is the document's info.version, the version of the API it is.
	Version string
	// Operations are the document's operations, by their paths in order and the method within each.
	Operations []Operation
	// Webhooks are the schema of the body each event posts, by the event's name.
	Webhooks map[string]map[string]any
}

// Operation is one operation of the document.
type Operation struct {
	ID     string
	Method string
	Path   string
	// Success is the first status of 2xx it answers.
	Success int

	parameters []parameter
	body       map[string]any
	pattern    *regexp.Regexp
	names      []string
	responses  map[string]any
}

type parameter struct {
	name     string
	in       string
	required bool
	schema   map[string]any
}

var methods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

var bearer = regexp.MustCompile(`^Bearer \S+$`)

// Load reads the document at path.
func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	document := &Document{raw: raw, Webhooks: map[string]map[string]any{}}
	info, _ := raw["info"].(map[string]any)
	document.Version, _ = info["version"].(string)

	servers, _ := raw["servers"].([]any)
	if len(servers) == 0 {
		return nil, fmt.Errorf("%s names no server", path)
	}
	first, _ := servers[0].(map[string]any)
	address, _ := first["url"].(string)
	if document.server, err = url.Parse(address); err != nil {
		return nil, fmt.Errorf("%s's server: %w", path, err)
	}

	paths, _ := raw["paths"].(map[string]any)
	for _, route := range sortedKeys(paths) {
		item, _ := paths[route].(map[string]any)
		for _, method := range methods {
			entry, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			operation, err := document.operation(route, method, item, entry)
			if err != nil {
				return nil, err
			}
			document.Operations = append(document.Operations, operation)
		}
	}

	webhooks, _ := raw["webhooks"].(map[string]any)
	for name, value := range webhooks {
		item, _ := value.(map[string]any)
		post, _ := item["post"].(map[string]any)
		schema, err := document.bodySchema(post)
		if err != nil || schema == nil {
			return nil, fmt.Errorf("the webhook %s posts no JSON body the document gives: %v", name, err)
		}
		document.Webhooks[name] = schema
	}
	return document, nil
}

func (d *Document) operation(route, method string, item, entry map[string]any) (Operation, error) {
	operation := Operation{Method: strings.ToUpper(method), Path: route}
	operation.ID, _ = entry["operationId"].(string)

	declared := map[string]parameter{}
	var order []string
	for _, owner := range []map[string]any{item, entry} {
		list, _ := owner["parameters"].([]any)
		for _, value := range list {
			found, err := d.resolve(value)
			if err != nil {
				return operation, fmt.Errorf("%s: %w", operation.ID, err)
			}
			p := parameter{}
			p.name, _ = found["name"].(string)
			p.in, _ = found["in"].(string)
			p.required, _ = found["required"].(bool)
			p.schema, _ = found["schema"].(map[string]any)
			key := p.in + ":" + strings.ToLower(p.name)
			if _, seen := declared[key]; !seen {
				order = append(order, key)
			}
			declared[key] = p
		}
	}
	for _, key := range order {
		operation.parameters = append(operation.parameters, declared[key])
	}

	body, err := d.bodySchema(entry)
	if err != nil {
		return operation, fmt.Errorf("%s: %w", operation.ID, err)
	}
	operation.body = body

	operation.responses, _ = entry["responses"].(map[string]any)
	for _, status := range sortedKeys(operation.responses) {
		if code, err := strconv.Atoi(status); err == nil && code >= 200 && code < 300 {
			operation.Success = code
			break
		}
	}

	// The path on the document's server, each {name} in it a segment of its own.
	for _, match := range pathParameter.FindAllStringSubmatch(route, -1) {
		operation.names = append(operation.names, match[1])
	}
	parts := pathParameter.Split(strings.TrimSuffix(d.server.Path, "/")+route, -1)
	for index, part := range parts {
		parts[index] = regexp.QuoteMeta(part)
	}
	operation.pattern = regexp.MustCompile("^" + strings.Join(parts, "([^/]+)") + "$")
	return operation, nil
}

// pathParameter is a parameter's place in a path, {fax_id}.
var pathParameter = regexp.MustCompile(`\{([^}]+)\}`)

// bodySchema answers the schema of an operation's JSON body, or nil where it takes none.
func (d *Document) bodySchema(entry map[string]any) (map[string]any, error) {
	value, ok := entry["requestBody"]
	if !ok {
		return nil, nil
	}
	body, err := d.resolve(value)
	if err != nil {
		return nil, err
	}
	content, _ := body["content"].(map[string]any)
	media, ok := content["application/json"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("its body is not JSON")
	}
	schema, _ := media["schema"].(map[string]any)
	return schema, nil
}

// resolve follows the $ref a value holds, if any, to the object it names in the document.
func (d *Document) resolve(value any) (map[string]any, error) {
	object, _ := value.(map[string]any)
	for {
		ref, ok := object["$ref"].(string)
		if !ok {
			return object, nil
		}
		target, err := d.pointer(ref)
		if err != nil {
			return nil, err
		}
		object = target
	}
}

// pointer answers the object a local reference, #/components/schemas/Fax, names.
func (d *Document) pointer(ref string) (map[string]any, error) {
	if !strings.HasPrefix(ref, "#/") {
		return nil, fmt.Errorf("%s is no reference into the document", ref)
	}
	var value any = d.raw
	for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s names nothing in the document", ref)
		}
		value = object[part]
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s names nothing in the document", ref)
	}
	return object, nil
}

// find answers the operation a request is, by its method and path on the document's server, with
// the values of its path's parameters.
func (d *Document) find(request *http.Request) (Operation, map[string]string, bool) {
	if request.URL.Scheme != d.server.Scheme || request.URL.Host != d.server.Host {
		return Operation{}, nil, false
	}
	for _, operation := range d.Operations {
		if operation.Method != request.Method {
			continue
		}
		match := operation.pattern.FindStringSubmatch(request.URL.EscapedPath())
		if match == nil {
			continue
		}
		values := map[string]string{}
		for index, name := range operation.names {
			value, err := url.PathUnescape(match[index+1])
			if err != nil {
				value = match[index+1]
			}
			values[name] = value
		}
		return operation, values, true
	}
	return Operation{}, nil, false
}

// ReadRequest reads a request against the document: the id of the operation it is, and each thing
// in it the document refuses, none where it fits. body is the request's body as it was sent, and
// version the version its Xip-Version must name.
func (d *Document) ReadRequest(request *http.Request, body []byte, version string) (string, []string) {
	operation, values, ok := d.find(request)
	if !ok {
		return "", []string{fmt.Sprintf("%s %s is no operation of the document", request.Method, request.URL)}
	}
	var refused []string
	if keys := request.Header.Values("Authorization"); len(keys) != 1 || !bearer.MatchString(keys[0]) {
		refused = append(refused, fmt.Sprintf("Authorization is %q, not one bearer key", keys))
	}
	if versions := request.Header.Values("Xip-Version"); len(versions) != 1 || versions[0] != version {
		refused = append(refused, fmt.Sprintf("Xip-Version is %q, not %s", versions, version))
	}

	query := request.URL.Query()
	takesQuery := map[string]bool{}
	for _, p := range operation.parameters {
		var given []string
		switch p.in {
		case "query":
			takesQuery[p.name] = true
			given = query[p.name]
		case "header":
			given = request.Header.Values(p.name)
		case "path":
			if value, ok := values[p.name]; ok {
				given = []string{value}
			}
		}
		at := p.in + " " + p.name
		switch {
		case len(given) == 0:
			if p.required {
				refused = append(refused, at+" is missing")
			}
		case len(given) > 1:
			refused = append(refused, fmt.Sprintf("%s is sent %d times", at, len(given)))
		default:
			refused = append(refused, d.checkText(p.schema, given[0], at)...)
		}
	}
	for _, name := range sortedKeys(map[string][]string(query)) {
		if !takesQuery[name] {
			refused = append(refused, fmt.Sprintf("query %s is no parameter of %s", name, operation.ID))
		}
	}

	switch {
	case operation.body != nil:
		if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
			refused = append(refused, "the body is not sent as JSON")
		}
		var value any
		if err := json.Unmarshal(body, &value); err != nil {
			refused = append(refused, fmt.Sprintf("the body is no JSON: %v", err))
		} else {
			refused = append(refused, d.Check(operation.body, value, "body")...)
		}
	case len(body) > 0:
		refused = append(refused, "a body the operation takes none of")
	}
	return operation.ID, refused
}

// ReadAnswer reads an answer to a request against the document: what in it the operation's answer
// for its status refuses, none where it fits. An empty body is an answer that carries none.
func (d *Document) ReadAnswer(request *http.Request, status int, body []byte) []string {
	operation, _, ok := d.find(request)
	if !ok {
		return []string{fmt.Sprintf("%s %s is no operation of the document", request.Method, request.URL)}
	}
	answer, ok := operation.responses[strconv.Itoa(status)]
	if !ok {
		return []string{fmt.Sprintf("%s gives no %d", operation.ID, status)}
	}
	response, err := d.resolve(answer)
	if err != nil {
		return []string{err.Error()}
	}
	content, _ := response["content"].(map[string]any)
	if content == nil {
		if len(body) > 0 {
			return []string{fmt.Sprintf("%s's %d carries no body", operation.ID, status)}
		}
		return nil
	}
	media, _ := content["application/json"].(map[string]any)
	schema, _ := media["schema"].(map[string]any)
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return []string{fmt.Sprintf("%s's %d is no JSON: %v", operation.ID, status, err)}
	}
	return d.Check(schema, value, "answer")
}

// VersionRefused answers what the operation's Xip-Version parameter refuses in version, none where
// it takes it.
func (d *Document) VersionRefused(operation Operation, version string) []string {
	for _, p := range operation.parameters {
		if p.in == "header" && p.name == "Xip-Version" {
			return d.checkText(p.schema, version, "header Xip-Version")
		}
	}
	return []string{operation.ID + " takes no Xip-Version"}
}

// checkText checks a parameter's text, read as its schema's type, against the schema.
func (d *Document) checkText(schema map[string]any, text, at string) []string {
	target, err := d.resolve(schema)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", at, err)}
	}
	if target["type"] == "integer" {
		number, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return []string{fmt.Sprintf("%s is %q, no integer", at, text)}
		}
		return d.Check(schema, float64(number), at)
	}
	return d.Check(schema, text, at)
}

// annotations are the keywords that describe a value without constraining it.
var annotations = map[string]bool{"description": true, "title": true, "default": true, "examples": true}

// Check answers what the schema refuses in value, a value as encoding/json decodes JSON into an any,
// each at its place from at; none where it fits.
func (d *Document) Check(schema map[string]any, value any, at string) []string {
	var refused []string
	for _, keyword := range sortedKeys(schema) {
		rule := schema[keyword]
		switch keyword {
		case "$ref":
			target, err := d.pointer(fmt.Sprint(rule))
			if err != nil {
				refused = append(refused, fmt.Sprintf("%s: %v", at, err))
				continue
			}
			refused = append(refused, d.Check(target, value, at)...)
		case "type":
			if !hasType(rule, value) {
				refused = append(refused, fmt.Sprintf("%s is %s, not %v", at, typeOf(value), rule))
			}
		case "enum":
			if !inList(rule, value) {
				refused = append(refused, fmt.Sprintf("%s is %v, none of %v", at, value, rule))
			}
		case "const":
			if !reflect.DeepEqual(rule, value) {
				refused = append(refused, fmt.Sprintf("%s is %v, not %v", at, value, rule))
			}
		case "minLength", "maxLength":
			text, ok := value.(string)
			limit, isNumber := rule.(float64)
			if !ok || !isNumber {
				continue
			}
			length := float64(utf8.RuneCountInString(text))
			if (keyword == "minLength" && length < limit) || (keyword == "maxLength" && length > limit) {
				refused = append(refused, fmt.Sprintf("%s has %v characters, past its %s of %v", at, length, keyword, limit))
			}
		case "pattern":
			text, ok := value.(string)
			if !ok {
				continue
			}
			expression, err := regexp.Compile(fmt.Sprint(rule))
			if err != nil {
				refused = append(refused, fmt.Sprintf("%s: its pattern %v doesn't compile: %v", at, rule, err))
			} else if !expression.MatchString(text) {
				refused = append(refused, fmt.Sprintf("%s is %q, not of the pattern %v", at, text, rule))
			}
		case "minimum", "maximum":
			number, ok := value.(float64)
			limit, isNumber := rule.(float64)
			if !ok || !isNumber {
				continue
			}
			if (keyword == "minimum" && number < limit) || (keyword == "maximum" && number > limit) {
				refused = append(refused, fmt.Sprintf("%s is %v, past its %s of %v", at, number, keyword, limit))
			}
		case "minItems", "maxItems":
			list, ok := value.([]any)
			limit, isNumber := rule.(float64)
			if !ok || !isNumber {
				continue
			}
			count := float64(len(list))
			if (keyword == "minItems" && count < limit) || (keyword == "maxItems" && count > limit) {
				refused = append(refused, fmt.Sprintf("%s has %v items, past its %s of %v", at, count, keyword, limit))
			}
		case "items":
			list, ok := value.([]any)
			items, isSchema := rule.(map[string]any)
			if !ok || !isSchema {
				continue
			}
			for index, item := range list {
				refused = append(refused, d.Check(items, item, fmt.Sprintf("%s[%d]", at, index))...)
			}
		case "properties":
			object, ok := value.(map[string]any)
			properties, isMap := rule.(map[string]any)
			if !ok || !isMap {
				continue
			}
			for _, name := range sortedKeys(properties) {
				member, present := object[name]
				property, isSchema := properties[name].(map[string]any)
				if present && isSchema {
					refused = append(refused, d.Check(property, member, at+"."+name)...)
				}
			}
		case "required":
			object, ok := value.(map[string]any)
			names, isList := rule.([]any)
			if !ok || !isList {
				continue
			}
			for _, name := range names {
				if _, present := object[fmt.Sprint(name)]; !present {
					refused = append(refused, fmt.Sprintf("%s.%v is missing", at, name))
				}
			}
		case "additionalProperties":
			object, ok := value.(map[string]any)
			if !ok {
				continue
			}
			properties, _ := schema["properties"].(map[string]any)
			for _, name := range sortedKeys(object) {
				if _, named := properties[name]; named {
					continue
				}
				switch extra := rule.(type) {
				case bool:
					if !extra {
						refused = append(refused, fmt.Sprintf("%s.%s is no member it takes", at, name))
					}
				case map[string]any:
					refused = append(refused, d.Check(extra, object[name], at+"."+name)...)
				}
			}
		case "propertyNames":
			object, ok := value.(map[string]any)
			names, isSchema := rule.(map[string]any)
			if !ok || !isSchema {
				continue
			}
			for _, name := range sortedKeys(object) {
				refused = append(refused, d.Check(names, name, at+" name "+name)...)
			}
		case "anyOf":
			members, _ := rule.([]any)
			fits := false
			for _, member := range members {
				schema, isSchema := member.(map[string]any)
				if isSchema && len(d.Check(schema, value, at)) == 0 {
					fits = true
					break
				}
			}
			if !fits {
				refused = append(refused, fmt.Sprintf("%s fits none of its anyOf", at))
			}
		default:
			if !annotations[keyword] {
				refused = append(refused, fmt.Sprintf("%s: the contract reads no %s, which its schema gives", at, keyword))
			}
		}
	}
	return refused
}

// hasType reports whether value is of the type, or one of the types, a schema's type names.
func hasType(rule, value any) bool {
	switch named := rule.(type) {
	case string:
		return isType(named, value)
	case []any:
		for _, name := range named {
			if isType(fmt.Sprint(name), value) {
				return true
			}
		}
	}
	return false
}

func isType(name string, value any) bool {
	switch name {
	case "null":
		return value == nil
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok := value.(float64)
		return ok
	case "integer":
		number, ok := value.(float64)
		return ok && number == math.Trunc(number)
	case "array":
		_, ok := value.([]any)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	}
	return false
}

func typeOf(value any) string {
	for _, name := range []string{"null", "boolean", "integer", "number", "string", "array", "object"} {
		if isType(name, value) {
			return name
		}
	}
	return fmt.Sprintf("%T", value)
}

func inList(rule, value any) bool {
	list, _ := rule.([]any)
	for _, allowed := range list {
		if reflect.DeepEqual(allowed, value) {
			return true
		}
	}
	return false
}

func sortedKeys[V any](object map[string]V) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
