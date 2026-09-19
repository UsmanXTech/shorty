package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type utmTestResponse struct {
	URL string `json:"url"`
}

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

	var got utmTestResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := "https://example.com/docs?utm_campaign=spring+launch&utm_medium=cpc&utm_source=google&x=1"
	if got.URL != want {
		t.Fatalf("url = %q, want %q", got.URL, want)
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
