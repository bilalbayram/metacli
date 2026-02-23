package testutil

import (
	"net/http"
	"net/http/httptest"
)

// NewJSONServer creates an httptest server from a callback to keep integration tests concise.
func NewJSONServer(handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(handler))
}
