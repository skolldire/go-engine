package errors

import (
	"context"
	"unicode"
	"unicode/utf8"
)

type wireProblem struct {
	Type      string `json:"type"`
	Status    int    `json:"status"`
	Title     string `json:"title"`
	Detail    string `json:"detail,omitempty"`
	Code      string `json:"code"`
	RequestID string `json:"request_id,omitempty"`
}

func validProblem(p Problem) bool {
	if p.Status < 400 || p.Status > 599 || len(p.Code) == 0 || len(p.Code) > 64 || len(p.Title) == 0 || len(p.Title) > 256 || !utf8.ValidString(p.Title) {
		return false
	}
	for i := range len(p.Code) {
		c := p.Code[i]
		allowed := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-'
		if !allowed {
			return false
		}
	}
	return true
}
func (r *Responder) prepare(ctx context.Context, original error) (wireProblem, error) {
	p := r.deps.Classify(original)
	var err error
	if !validProblem(p) {
		p = Problem{Status: 500, Code: "internal_error", Title: "Internal Server Error"}
		err = ErrInvalidProblem
	}
	out := wireProblem{Type: "about:blank", Status: p.Status, Code: p.Code, Title: p.Title}
	if len(p.Details) <= r.cfg.MaxDetailsBytes && utf8.ValidString(p.Details) {
		out.Detail = p.Details
	}
	if r.cfg.IncludeRequestID && r.deps.RequestID != nil {
		id := r.deps.RequestID(ctx)
		if validRequestID(id) {
			out.RequestID = id
		}
	}
	return out, err
}

func validRequestID(id string) bool {
	if len(id) > 128 || !utf8.ValidString(id) {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
