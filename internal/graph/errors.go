package graph

import "fmt"

type APIError struct {
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	Message      string `json:"message"`
	FBTraceID    string `json:"fbtrace_id"`
	Retryable    bool   `json:"retryable"`
	StatusCode   int    `json:"-"`
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf(
		"meta api error type=%s code=%d subcode=%d fbtrace_id=%s: %s",
		e.Type,
		e.Code,
		e.ErrorSubcode,
		e.FBTraceID,
		e.Message,
	)
}

type TransientError struct {
	Message    string
	StatusCode int
}

func (e *TransientError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}
