package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/database"
)

func testStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewSQLiteStore(db.DB)
}

func TestStoreCRUD(t *testing.T) {
	s := testStore(t)
	hook, err := s.Create(1, "https://example.com/hook", []string{EventLinkCreated})
	if err != nil {
		t.Fatal(err)
	}
	if hook.Secret == "" {
		t.Fatal("expected a signing secret")
	}
	if _, err := s.Create(1, "https://example.com/hook", []string{"bogus.event"}); err == nil {
		t.Fatal("expected error for unknown event")
	}

	active, err := s.ListActive(1, EventLinkCreated)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active hook, got %d", len(active))
	}
	if active, _ := s.ListActive(1, EventLinkClicked); len(active) != 0 {
		t.Fatalf("expected no hooks for link.clicked, got %d", len(active))
	}

	updated, err := s.Update(hook.ID, "https://example.com/hook2", []string{EventLinkClicked}, false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.URL != "https://example.com/hook2" || updated.Active {
		t.Fatalf("unexpected update: %+v", updated)
	}
	if active, _ := s.ListActive(1, EventLinkClicked); len(active) != 0 {
		t.Fatal("inactive hook must not be listed")
	}

	if err := s.Delete(hook.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(hook.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDispatcherDeliversSignedPayload(t *testing.T) {
	s := testStore(t)

	var gotEvent, gotSig string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEvent = r.Header.Get("X-Shorty-Event")
		gotSig = r.Header.Get("X-Shorty-Signature")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hook, err := s.Create(1, srv.URL, []string{EventLinkClicked})
	if err != nil {
		t.Fatal(err)
	}

	d := NewDispatcherWithClient(s, 1, &http.Client{Timeout: 8 * time.Second})
	defer d.Close()
	d.Emit(1, EventLinkClicked, map[string]any{"slug": "go"})

	deadline := time.Now().Add(5 * time.Second)
	for {
		deliveries, _ := s.ListDeliveries(hook.ID, 10)
		if len(deliveries) > 0 {
			d0 := deliveries[0]
			if !d0.Success || d0.StatusCode == nil || *d0.StatusCode != 200 {
				t.Fatalf("unexpected delivery: %+v", d0)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for delivery")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if gotEvent != EventLinkClicked {
		t.Fatalf("expected event header %q, got %q", EventLinkClicked, gotEvent)
	}
	if !strings.HasPrefix(gotSig, "sha256=") {
		t.Fatalf("expected sha256 signature, got %q", gotSig)
	}
	mac := hmac.New(sha256.New, []byte(hook.Secret))
	mac.Write(gotBody)
	if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); want != gotSig {
		t.Fatal("signature mismatch")
	}
	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["event"] != EventLinkClicked {
		t.Fatalf("unexpected payload event: %v", payload["event"])
	}

	// Emitting an event nobody subscribed to must not error or block.
	d.Emit(1, EventLinkExpired, map[string]any{"slug": "go"})
}

func TestDispatcherRecordsFailure(t *testing.T) {
	s := testStore(t)
	hook, err := s.Create(1, "http://127.0.0.1:1/unreachable", []string{EventLinkCreated})
	if err != nil {
		t.Fatal(err)
	}
	d := NewDispatcherWithClient(s, 1, &http.Client{Timeout: 8 * time.Second})
	defer d.Close()
	d.Emit(1, EventLinkCreated, map[string]any{"slug": "go"})

	deadline := time.Now().Add(10 * time.Second)
	for {
		deliveries, _ := s.ListDeliveries(hook.ID, 10)
		if len(deliveries) > 0 {
			if deliveries[0].Success {
				t.Fatal("expected failed delivery")
			}
			if deliveries[0].Error == "" {
				t.Fatal("expected error text")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for delivery")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestValidateURL(t *testing.T) {
	bad := []string{
		"",
		"not-a-url",
		"ftp://93.184.216.34/hook",
		"http://127.0.0.1/hook",
		"http://10.0.0.5/hook",
		"http://172.16.0.5/hook",
		"http://192.168.1.5/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.64.0.1/hook",
		"http://[::1]/hook",
		"https://user:pass@93.184.216.34/hook",
		"https://0.0.0.0/hook",
	}
	for _, raw := range bad {
		if err := ValidateURL(raw); err == nil {
			t.Fatalf("expected ValidateURL(%q) to fail", raw)
		}
	}
	good := []string{
		"https://93.184.216.34/hook",
		"http://8.8.8.8/hook",
	}
	for _, raw := range good {
		if err := ValidateURL(raw); err != nil {
			t.Fatalf("expected ValidateURL(%q) to pass, got %v", raw, err)
		}
	}
}

func TestDialPublicIPBlocksNonPublic(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:80", "10.1.2.3:443", "169.254.169.254:80"} {
		if _, err := dialPublicIP(context.Background(), "tcp", addr); err == nil {
			t.Fatalf("expected dialPublicIP(%q) to be blocked", addr)
		}
	}
	if _, err := dialPublicIP(context.Background(), "udp", "8.8.8.8:53"); err == nil {
		t.Fatal("expected non-tcp dial to be blocked")
	}
}
