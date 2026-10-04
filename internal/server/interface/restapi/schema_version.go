package restapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kipitix/growscada/contract"
	apiv0 "github.com/kipitix/growscada/contract/api/v0"
)

// schemaVersionMiddleware stamps every response with the SchemaVersion of the
// server API, so a client can tell which MINOR it is talking to (ADR 0005).
func schemaVersionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(contract.SchemaVersionHeader, apiv0.SchemaVersion().String())
		next.ServeHTTP(w, r)
	})
}

// decodeRequest decodes a request body strictly into v and reports whether it
// succeeded; on failure it has already answered 400. The server keeps what a
// request carries, so anything it would otherwise silently drop is an error
// (ADR 0005): a field it does not know, a key given twice (encoding/json
// matches keys case-insensitively and keeps the last), data after the value.
// When the client states a newer SchemaVersion and the error is an unknown
// field, the error says so instead of only naming the field.
func decodeRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	err := decodeStrict(r.Body, v)
	if err == nil {
		return true
	}
	detail := err.Error()
	if isUnknownField(err) {
		if client, err := contract.ParseSchemaVersion(r.Header.Get(contract.SchemaVersionHeader)); err == nil &&
			client.NewerThan(apiv0.SchemaVersion()) {
			detail = fmt.Sprintf("the client speaks server API %v, newer than this server's %v: update the server (%s)",
				client, apiv0.SchemaVersion(), detail)
		}
	}
	sendJSONResponse(w, http.StatusBadRequest, NewBadRequest(detail, r.URL.Path))
	return false
}

// isUnknownField reports whether err is DisallowUnknownFields' error, which
// encoding/json does not export as a type.
func isUnknownField(err error) bool {
	return strings.HasPrefix(err.Error(), "json: unknown field ")
}

func decodeStrict(body io.Reader, v any) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if err := checkDuplicateKeys(data); err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("json: request body must hold a single JSON value")
	}
	return nil
}

type duplicateKeyError struct{ key string }

func (e *duplicateKeyError) Error() string { return fmt.Sprintf("json: duplicate key %q", e.key) }

// checkDuplicateKeys rejects an object that names the same key twice, compared
// case-insensitively as encoding/json matches keys to fields. Syntax errors are
// left to the decoder, which reports them with more context.
func checkDuplicateKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok {
		case json.Delim('{'):
			seen := map[string]bool{}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key := strings.ToLower(keyTok.(string))
				if seen[key] {
					return &duplicateKeyError{key: keyTok.(string)}
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		case json.Delim('['):
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		}
		return nil
	}
	var dup *duplicateKeyError
	if err := walk(); err != nil && errors.As(err, &dup) {
		return err
	}
	return nil
}
