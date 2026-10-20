package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/trustline/trustline/internal/domain"
)

type errBody struct {
	Error struct {
		Code    string            `json:"code"`
		Message string            `json:"message"`
		Fields  map[string]string `json:"fields,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	var b errBody
	b.Error.Code = code
	b.Error.Message = msg
	writeJSON(w, status, b)
}

func writeValidation(w http.ResponseWriter, v *domain.ValidationError) {
	var b errBody
	b.Error.Code = "validation_failed"
	b.Error.Message = "some fields are invalid"
	b.Error.Fields = v.Fields
	writeJSON(w, 422, b)
}

// fail maps domain errors to HTTP responses.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var v *domain.ValidationError
	switch {
	case errors.As(err, &v):
		writeValidation(w, v)
	case errors.Is(err, domain.ErrNotFound), errors.Is(err, domain.ErrInvalidID):
		writeError(w, 404, "not_found", "not found")
	case errors.Is(err, domain.ErrConflict):
		writeError(w, 409, "conflict", "already exists")
	case errors.Is(err, domain.ErrUnauthorized):
		writeError(w, 401, "unauthorized", "sign in required")
	case errors.Is(err, domain.ErrForbidden):
		writeError(w, 403, "forbidden", "you do not have permission to do this")
	case errors.Is(err, domain.ErrRateLimited):
		writeError(w, 429, "rate_limited", "too many attempts, try again later")
	default:
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
		writeError(w, 500, "internal", "something went wrong")
	}
}

func decode(r *http.Request, v any) error { return decodeLimit(r, v, 1<<20) }

func decodeLimit(r *http.Request, v any, limit int64) error {
	r.Body = http.MaxBytesReader(nil, r.Body, limit)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return domain.Invalid("body", "request body is required")
		}
		return domain.Invalid("body", "invalid JSON: "+err.Error())
	}
	return nil
}
