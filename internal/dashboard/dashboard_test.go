package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesDashboard(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	Handler().ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusOK)
	}
	if got := res.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("content type = %q, want text/html", got)
	}
	body := res.Body.String()
	if !strings.Contains(body, "Shorty Dashboard") {
		t.Fatal("dashboard title missing")
	}
	for _, heading := range []string{"Click activity", "Referrers", "Countries", "Browsers", "Devices"} {
		if !strings.Contains(body, ">"+heading+"<") {
			t.Fatalf("dashboard heading %q missing", heading)
		}
	}
	if !strings.Contains(body, "function drawChart") || !strings.Contains(body, "function drawBreakdown") {
		t.Fatal("dashboard chart rendering functions missing")
	}
}

func TestHandlerRejectsNonRootPaths(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	res := httptest.NewRecorder()
	Handler().ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", res.Code, http.StatusNotFound)
	}
}
