package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUTMAPI(t *testing.T) {
	mux := http.NewServeMux()
	NewUTMAPI().Routes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/utm", strings.NewReader(`{"url":"https://example.com/docs?x=1","utm_source":"google","utm_medium":"cpc","utm_campaign":"spring launch"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	want := `{"url":"https://example.com/docs?utm_campaign=spring+launch&utm_medium=cpc&utm_source=google&x=1"}`
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("body = %q, want %q", strings.TrimSpace(rec.Body.String()), want)
	}
}

func TestUTMAPIRejectsInvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	NewUTMAPI().Routes(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/utm", strings.NewReader("{"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
