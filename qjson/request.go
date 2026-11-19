package qjson

import (
	"mime"
	"net/http"
	"strings"
)

// DecodeRequest reads a JSON request body into a new T and checks its
// Content-Type first. It accepts application/json and any application/*+json
// media type; use AnyContentType when the caller is allowed to omit or vary it.
//
// The body limit and strictness are the ones of Decode.
func DecodeRequest[T any](r *http.Request, opts ...Option) (T, error) {
	var zero T
	cfg := defaultConfig()
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	if r == nil || r.Body == nil {
		return zero, &Error{Code: CodeEmptyBody, Msg: "the request has no body"}
	}
	if !cfg.anyContentType {
		ct := r.Header.Get("Content-Type")
		if !isJSONMediaType(ct) {
			msg := "expected application/json"
			if ct != "" {
				msg = "Content-Type is " + ct + ", expected application/json"
			}
			return zero, &Error{Code: CodeUnsupportedMediaType, Msg: msg}
		}
	}
	return Decode[T](r.Body, opts...)
}

func isJSONMediaType(v string) bool {
	if v == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(v)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}
