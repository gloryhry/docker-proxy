package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRestoreOriginalPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/index?__path=/v2/&foo=bar", nil)
	recorder := httptest.NewRecorder()

	Handler(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", recorder.Code)
	}
	if got := recorder.Header().Get("Docker-Distribution-Api-Version"); got != "registry/2.0" {
		t.Fatalf("unexpected version header: %s", got)
	}
}
