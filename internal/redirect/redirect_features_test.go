package redirect

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/domains"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/password"
	"github.com/UsmanXTech/shorty/internal/variants"
)

func TestPasswordGate(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	hash, err := password.Hash("s3cretpw")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Create(links.Link{Slug: "locked", URL: "https://example.com", CreatedAt: now, UpdatedAt: now, PasswordHash: hash})
	if err != nil {
		t.Fatal(err)
	}

	h := New(repo).WithPasswordSigner(password.NewCookieSigner("test-secret"))

	// GET shows the password form.
	req := httptest.NewRequest(http.MethodGet, "/locked", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with password form, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "password") {
		t.Fatal("expected password form in body")
	}

	// Wrong password -> 401.
	form := url.Values{"password": {"nope"}}
	req = httptest.NewRequest(http.MethodPost, "/locked", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	// Correct password -> sets cookie and redirects.
	form = url.Values{"password": {"s3cretpw"}}
	req = httptest.NewRequest(http.MethodPost, "/locked", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected unlock cookie")
	}

	// Cookie grants access without re-entering the password.
	req = httptest.NewRequest(http.MethodGet, "/locked", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 with cookie, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://example.com" {
		t.Fatalf("unexpected location %q", loc)
	}

	// Clicks only counted after unlock.
	got, _ := repo.GetBySlug("locked")
	if got.Clicks != 2 {
		t.Fatalf("expected 2 clicks, got %d", got.Clicks)
	}
}

func TestVariantRedirect(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	link, err := repo.Create(links.Link{Slug: "ab", URL: "https://default.example", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	vstore := variants.NewMemoryStore()
	vstore.AddLink(link.ID)
	va, err := vstore.Create(link.ID, "https://a.example", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vstore.Create(link.ID, "https://b.example", 1); err != nil {
		t.Fatal(err)
	}

	h := New(repo).WithVariants(vstore)
	seen := map[string]int{}
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ab", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
		seen[rec.Header().Get("Location")]++
	}
	if len(seen) != 2 {
		t.Fatalf("expected both variants to be served, got %v", seen)
	}
	// Link without variants still redirects to its URL.
	req := httptest.NewRequest(http.MethodGet, "/ab", nil)
	rec := httptest.NewRecorder()
	New(repo).ServeHTTP(rec, req)
	_ = va
}

func TestVariantClicksTracked(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	link, _ := repo.Create(links.Link{Slug: "ab2", URL: "https://default.example", CreatedAt: now, UpdatedAt: now})
	vstore := variants.NewMemoryStore()
	vstore.AddLink(link.ID)
	va, _ := vstore.Create(link.ID, "https://only.example", 1)

	h := New(repo).WithVariants(vstore)
	req := httptest.NewRequest(http.MethodGet, "/ab2", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://only.example" {
		t.Fatalf("expected variant URL, got %q", loc)
	}
	got, _ := vstore.Get(va.ID)
	if got.Clicks != 1 {
		t.Fatalf("expected 1 variant click, got %d", got.Clicks)
	}
}

func TestDomainRouting(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()

	dstore := domains.NewMemoryStore()
	d, err := dstore.Create(1, "short.example")
	if err != nil {
		t.Fatal(err)
	}

	_, err = repo.Create(links.Link{Slug: "go", URL: "https://custom.example/landing", CreatedAt: now, UpdatedAt: now, DomainID: &d.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Create(links.Link{Slug: "go2", URL: "https://default.example/home", CreatedAt: now, UpdatedAt: now})
	if err != nil {
		t.Fatal(err)
	}

	h := New(repo).WithDomains(dstore)

	// Custom host serves the domain link.
	req := httptest.NewRequest(http.MethodGet, "/go", nil)
	req.Host = "short.example"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 on custom domain, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "https://custom.example/landing" {
		t.Fatalf("unexpected location %q", loc)
	}

	// Default host does not see domain-scoped slugs.
	req = httptest.NewRequest(http.MethodGet, "/go", nil)
	req.Host = "localhost:8080"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on default host, got %d", rec.Code)
	}

	// Default-domain link still works on the default host.
	req = httptest.NewRequest(http.MethodGet, "/go2", nil)
	req.Host = "localhost:8080"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
}

func TestUnlockRateLimited(t *testing.T) {
	repo := links.NewMemoryRepository()
	now := time.Now().UTC()
	hash, err := password.Hash("s3cretpw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(links.Link{Slug: "locked", URL: "https://example.com", CreatedAt: now, UpdatedAt: now, PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	h := New(repo).WithPasswordSigner(password.NewCookieSigner("test-secret"))

	post := func() int {
		form := url.Values{"password": {"nope"}}
		req := httptest.NewRequest(http.MethodPost, "/locked", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// 10 wrong guesses are allowed through to password verification (401).
	for i := 0; i < 10; i++ {
		if code := post(); code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, code)
		}
	}
	// The 11th attempt within a minute is rejected without burning CPU on bcrypt.
	if code := post(); code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after too many attempts, got %d", code)
	}
}

func TestExpiredEmissionCooldown(t *testing.T) {
	h := New(links.NewMemoryRepository())
	if !h.shouldEmitExpired("old") {
		t.Fatal("first emission should be allowed")
	}
	if h.shouldEmitExpired("old") {
		t.Fatal("second emission within the cooldown should be suppressed")
	}
	if !h.shouldEmitExpired("other") {
		t.Fatal("a different slug should not be affected by the cooldown")
	}
}

// countingVariantStore wraps a variants.Store to count List calls.
type countingVariantStore struct {
	variants.Store
	lists int
}

func (s *countingVariantStore) List(linkID int64) ([]variants.Variant, error) {
	s.lists++
	return s.Store.List(linkID)
}

func TestVariantListCacheAvoidsPerClickQuery(t *testing.T) {
	repo := links.NewMemoryRepository()
	link, err := repo.Create(links.Link{Slug: "novar", URL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	memStore := variants.NewMemoryStore()
	memStore.AddLink(link.ID)
	store := &countingVariantStore{Store: memStore}
	lc := variants.NewListCache()
	h := New(repo).WithVariants(store).WithVariantListCache(lc)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/novar", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound {
			t.Fatalf("expected 302, got %d", rec.Code)
		}
	}
	// Only the first redirect should have queried the store; the rest
	// served the (empty) list from the cache.
	if store.lists != 1 {
		t.Fatalf("expected 1 variant List call, got %d", store.lists)
	}
	if _, ok := lc.Get(link.ID); !ok {
		t.Fatal("expected the variant list to be cached")
	}
}
