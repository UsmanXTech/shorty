package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/UsmanXTech/shorty/internal/database"
	"github.com/UsmanXTech/shorty/internal/domains"
	"github.com/UsmanXTech/shorty/internal/links"
	"github.com/UsmanXTech/shorty/internal/teams"
	"github.com/UsmanXTech/shorty/internal/variants"
	"github.com/UsmanXTech/shorty/internal/webhooks"
)

func testDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "shorty.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func createTestLink(t *testing.T, mux http.Handler, body string) links.Link {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var link links.Link
	if err := json.NewDecoder(rec.Body).Decode(&link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestVariantAPILifecycle(t *testing.T) {
	repo := links.NewMemoryRepository()
	vstore := variants.NewMemoryStore()
	mux := http.NewServeMux()
	NewLinkAPI(repo).Routes(mux)
	NewVariantAPI(repo, vstore).Routes(mux)

	link := createTestLink(t, mux, `{"url":"https://example.com","slug":"abtest"}`)
	vstore.AddLink(link.ID)
	base := "/api/v1/links/" + strconv.FormatInt(link.ID, 10) + "/variants"

	// Create.
	req := httptest.NewRequest(http.MethodPost, base, strings.NewReader(`{"url":"https://b.example","weight":3}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var v variants.Variant
	_ = json.NewDecoder(rec.Body).Decode(&v)
	if v.Weight != 3 || v.URL != "https://b.example" {
		t.Fatalf("unexpected variant: %+v", v)
	}

	// List.
	req = httptest.NewRequest(http.MethodGet, base, nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var list []variants.Variant
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 variant, got %d", len(list))
	}

	// Reject bad URL.
	req = httptest.NewRequest(http.MethodPost, base, strings.NewReader(`{"url":"notaurl"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	// Update.
	req = httptest.NewRequest(http.MethodPut, base+"/"+strconv.FormatInt(v.ID, 10), strings.NewReader(`{"weight":7}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var updated variants.Variant
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated.Weight != 7 {
		t.Fatalf("expected weight 7, got %+v", updated)
	}

	// Delete.
	req = httptest.NewRequest(http.MethodDelete, base+"/"+strconv.FormatInt(v.ID, 10), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestDomainAPILifecycle(t *testing.T) {
	store := domains.NewMemoryStore()
	mux := http.NewServeMux()
	NewDomainAPI(store).Routes(mux)

	// Domain registration requires an authenticated API key.
	authed := func(r *http.Request) *http.Request {
		ctx := teams.WithTeam(r.Context(), teams.Team{ID: 1, Name: "Default"})
		ctx = teams.WithAPIKey(ctx, teams.APIKey{ID: 1, TeamID: 1})
		return r.WithContext(ctx)
	}

	// Anonymous create is rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(`{"domain":"anon.example"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anonymous domain create, got %d", rec.Code)
	}

	req = authed(httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(`{"domain":"Short.Example"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var d domains.Domain
	_ = json.NewDecoder(rec.Body).Decode(&d)
	if d.Domain != "short.example" {
		t.Fatalf("expected normalized domain, got %q", d.Domain)
	}

	// Duplicate.
	req = authed(httptest.NewRequest(http.MethodPost, "/api/v1/domains", strings.NewReader(`{"domain":"short.example"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}

	// List then delete.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/domains", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var list []domains.Domain
	_ = json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(list))
	}
	// Anonymous delete is rejected too.
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(d.ID, 10), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anonymous domain delete, got %d", rec.Code)
	}
	req = authed(httptest.NewRequest(http.MethodDelete, "/api/v1/domains/"+strconv.FormatInt(d.ID, 10), nil))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestWebhookAPILifecycle(t *testing.T) {
	db := testDB(t)
	store := webhooks.NewSQLiteStore(db.DB)
	mux := http.NewServeMux()
	webhookAPI := NewWebhookAPI(store)
	// Stub URL validation: sandbox DNS maps example.com to a reserved IP.
	webhookAPI.validateURL = func(string) error { return nil }
	webhookAPI.Routes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks",
		strings.NewReader(`{"url":"https://example.com/hook","events":["link.created"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var hook webhooks.Webhook
	_ = json.NewDecoder(rec.Body).Decode(&hook)
	if len(hook.Events) != 1 || hook.Events[0] != webhooks.EventLinkCreated {
		t.Fatalf("unexpected hook: %+v", hook)
	}
	// The signing secret is returned exactly once, at creation.
	if hook.PlaintextSecret == "" {
		t.Fatal("expected secret in creation response")
	}

	// Subsequent reads must not expose the secret.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+strconv.FormatInt(hook.ID, 10), nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var fetched webhooks.Webhook
	_ = json.NewDecoder(rec.Body).Decode(&fetched)
	if fetched.PlaintextSecret != "" {
		t.Fatal("secret must not be exposed on read")
	}

	// Rotation returns the new secret exactly once.
	req = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+strconv.FormatInt(hook.ID, 10)+"/rotate-secret", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var rotated webhooks.Webhook
	_ = json.NewDecoder(rec.Body).Decode(&rotated)
	if rotated.PlaintextSecret == "" || rotated.PlaintextSecret == hook.PlaintextSecret {
		t.Fatal("expected a fresh secret from rotation")
	}

	// Deliveries start empty.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/webhooks/"+strconv.FormatInt(hook.ID, 10)+"/deliveries", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Deactivate.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/webhooks/"+strconv.FormatInt(hook.ID, 10),
		strings.NewReader(`{"active":false}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var updated webhooks.Webhook
	_ = json.NewDecoder(rec.Body).Decode(&updated)
	if updated.Active {
		t.Fatal("expected webhook to be inactive")
	}
}

func TestTeamAPILifecycle(t *testing.T) {
	db := testDB(t)
	store := teams.NewSQLiteStore(db.DB)
	mux := http.NewServeMux()
	NewTeamAPI(store).Routes(mux)

	// Team management is admin-only; attach an admin key context to requests.
	adminCtx := func(req *http.Request) *http.Request {
		ctx := teams.WithTeam(req.Context(), teams.Team{ID: teams.DefaultTeamID, Name: "Default"})
		ctx = teams.WithAPIKey(ctx, teams.APIKey{ID: 1, TeamID: teams.DefaultTeamID, Name: "test-admin", IsAdmin: true})
		return req.WithContext(ctx)
	}

	// Anonymous team creation must be rejected.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"name":"Evil"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for anonymous team create, got %d", rec.Code)
	}

	req = adminCtx(httptest.NewRequest(http.MethodPost, "/api/v1/teams", strings.NewReader(`{"name":"Acme"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var team teams.Team
	_ = json.NewDecoder(rec.Body).Decode(&team)

	// Anonymous key listing must be rejected too.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+strconv.FormatInt(team.ID, 10)+"/keys", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for anonymous key list, got %d", rec.Code)
	}

	// Admins can mint admin keys; non-admin keys cannot.
	req = adminCtx(httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+strconv.FormatInt(team.ID, 10)+"/keys",
		strings.NewReader(`{"name":"ci","admin":true}`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var key teams.APIKey
	_ = json.NewDecoder(rec.Body).Decode(&key)
	if key.Plaintext == "" {
		t.Fatal("expected plaintext key in creation response")
	}
	if !key.IsAdmin {
		t.Fatal("expected admin key")
	}

	nonAdminCtx := func(req *http.Request) *http.Request {
		ctx := teams.WithTeam(req.Context(), teams.Team{ID: team.ID, Name: "Acme"})
		ctx = teams.WithAPIKey(ctx, teams.APIKey{ID: 2, TeamID: team.ID, Name: "plain", IsAdmin: false})
		return req.WithContext(ctx)
	}
	req = nonAdminCtx(httptest.NewRequest(http.MethodPost, "/api/v1/teams/"+strconv.FormatInt(team.ID, 10)+"/keys",
		strings.NewReader(`{"name":"sneaky","admin":true}`)))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin minting admin key, got %d", rec.Code)
	}

	req = adminCtx(httptest.NewRequest(http.MethodGet, "/api/v1/teams/"+strconv.FormatInt(team.ID, 10)+"/keys", nil))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var keyList []teams.APIKey
	_ = json.NewDecoder(rec.Body).Decode(&keyList)
	if len(keyList) != 1 || keyList[0].Plaintext != "" {
		t.Fatalf("plaintext must not leak on list: %+v", keyList)
	}
}

func TestWebhookAPIRejectsPrivateURL(t *testing.T) {
	db := testDB(t)
	store := webhooks.NewSQLiteStore(db.DB)
	mux := http.NewServeMux()
	NewWebhookAPI(store).Routes(mux) // real webhooks.ValidateURL

	for _, raw := range []string{
		`{"url":"http://127.0.0.1/hook"}`,
		`{"url":"http://10.1.2.3/hook"}`,
		`{"url":"http://169.254.169.254/hook"}`,
		`{"url":"ftp://93.184.216.34/hook"}`,
		`{"url":"https://user:pass@93.184.216.34/hook"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for %s, got %d", raw, rec.Code)
		}
	}
}

func TestLinkUpdateExpiresAtOmittedVsNull(t *testing.T) {
	repo := links.NewMemoryRepository()
	mux := http.NewServeMux()
	NewLinkAPI(repo).Routes(mux)

	future := time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339)
	link := createTestLink(t, mux, `{"url":"https://example.com","slug":"exp","expires_at":"`+future+`"}`)
	if link.ExpiresAt == nil {
		t.Fatal("expected expires_at to be set")
	}

	put := func(body string) links.Link {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/links/"+strconv.FormatInt(link.ID, 10), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var updated links.Link
		_ = json.NewDecoder(rec.Body).Decode(&updated)
		return updated
	}

	// Omitted expires_at must preserve the current value.
	if updated := put(`{"max_clicks":5}`); updated.ExpiresAt == nil {
		t.Fatal("omitted expires_at must not clear the existing value")
	}

	// Explicit null clears it.
	if updated := put(`{"expires_at":null}`); updated.ExpiresAt != nil {
		t.Fatal("explicit null expires_at must clear the value")
	}

	// A new value sets it again.
	if updated := put(`{"expires_at":"` + future + `"}`); updated.ExpiresAt == nil {
		t.Fatal("explicit expires_at must set the value")
	}
}

func TestLinkUpdateMaxClicksOmittedVsNull(t *testing.T) {
	repo := links.NewMemoryRepository()
	mux := http.NewServeMux()
	NewLinkAPI(repo).Routes(mux)

	link := createTestLink(t, mux, `{"url":"https://example.com","slug":"mc","max_clicks":5}`)
	if link.MaxClicks == nil || *link.MaxClicks != 5 {
		t.Fatal("expected max_clicks to be set to 5")
	}

	put := func(body string) links.Link {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/links/"+strconv.FormatInt(link.ID, 10), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var updated links.Link
		_ = json.NewDecoder(rec.Body).Decode(&updated)
		return updated
	}

	// Omitted max_clicks must preserve the current value.
	if updated := put(`{"slug":"mc"}`); updated.MaxClicks == nil || *updated.MaxClicks != 5 {
		t.Fatal("omitted max_clicks must not clear the existing value")
	}

	// Explicit null clears it.
	if updated := put(`{"max_clicks":null}`); updated.MaxClicks != nil {
		t.Fatal("explicit null max_clicks must clear the value")
	}

	// A new value sets it again.
	if updated := put(`{"max_clicks":9}`); updated.MaxClicks == nil || *updated.MaxClicks != 9 {
		t.Fatal("explicit max_clicks must set the value")
	}

	// Zero is rejected.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/links/"+strconv.FormatInt(link.ID, 10), strings.NewReader(`{"max_clicks":0}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for max_clicks=0, got %d", rec.Code)
	}
}

func TestLinkAPIPassword(t *testing.T) {
	repo := links.NewMemoryRepository()
	mux := http.NewServeMux()
	NewLinkAPI(repo).Routes(mux)

	link := createTestLink(t, mux, `{"url":"https://example.com","slug":"secret","password":"s3cretpw"}`)
	stored, err := repo.GetByID(link.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash == "" || stored.PasswordHash == "s3cretpw" {
		t.Fatal("password must be stored as a hash")
	}
	if !stored.Protected() {
		t.Fatal("expected link to be protected")
	}

	// Response must not leak the hash.
	var raw map[string]any
	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/"+strconv.FormatInt(link.ID, 10), nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	_ = json.NewDecoder(rec.Body).Decode(&raw)
	if _, ok := raw["PasswordHash"]; ok {
		t.Fatal("password hash leaked in API response")
	}

	// Clear the password via update.
	req = httptest.NewRequest(http.MethodPut, "/api/v1/links/"+strconv.FormatInt(link.ID, 10), strings.NewReader(`{"password":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	stored, _ = repo.GetByID(link.ID)
	if stored.Protected() {
		t.Fatal("expected protection to be cleared")
	}
}

func TestCSVExportImport(t *testing.T) {
	repo := links.NewMemoryRepository()
	mux := http.NewServeMux()
	NewLinkAPI(repo).Routes(mux)
	NewCSVAPI(repo).Routes(mux)

	now := time.Now().UTC()
	_, _ = repo.Create(links.Link{Slug: "one", URL: "https://one.example", CreatedAt: now, UpdatedAt: now})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/links/export", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv" {
		t.Fatalf("expected text/csv, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "one,https://one.example") {
		t.Fatalf("unexpected CSV body:\n%s", rec.Body.String())
	}

	// Import.
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "links.csv")
	_, _ = part.Write([]byte("slug,url,max_clicks\ntwo,https://two.example,5\nbad,notaurl\n"))
	_ = writer.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/links/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var summary map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&summary)
	if summary["created"] != float64(1) || summary["skipped"] != float64(1) {
		t.Fatalf("unexpected summary: %v", summary)
	}
	imported, err := repo.GetBySlug("two")
	if err != nil {
		t.Fatal(err)
	}
	if imported.MaxClicks == nil || *imported.MaxClicks != 5 {
		t.Fatalf("unexpected imported link: %+v", imported)
	}

	// Missing url column.
	body.Reset()
	writer = multipart.NewWriter(&body)
	part, _ = writer.CreateFormFile("file", "links.csv")
	_, _ = part.Write([]byte("slug\nfoo\n"))
	_ = writer.Close()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/links/import", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestVariantWeightSemantics(t *testing.T) {
	repo := links.NewMemoryRepository()
	vstore := variants.NewMemoryStore()
	mux := http.NewServeMux()
	NewLinkAPI(repo).Routes(mux)
	NewVariantAPI(repo, vstore).Routes(mux)

	link := createTestLink(t, mux, `{"url":"https://example.com","slug":"wtest"}`)
	vstore.AddLink(link.ID)
	base := "/api/v1/links/" + strconv.FormatInt(link.ID, 10) + "/variants"

	doReq := func(method, url, body string) (int, variants.Variant) {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var v variants.Variant
		_ = json.NewDecoder(rec.Body).Decode(&v)
		return rec.Code, v
	}

	// Omitted weight defaults to 1 on create.
	code, v := doReq(http.MethodPost, base, `{"url":"https://a.example"}`)
	if code != http.StatusCreated || v.Weight != 1 {
		t.Fatalf("expected 201 with weight 1, got %d weight %d", code, v.Weight)
	}
	// Explicit 0 disables the variant instead of silently no-opping.
	code, v = doReq(http.MethodPost, base, `{"url":"https://b.example","weight":0}`)
	if code != http.StatusCreated || v.Weight != 0 {
		t.Fatalf("expected 201 with weight 0, got %d weight %d", code, v.Weight)
	}
	vid := v.ID

	// Omitted weight on update preserves the current value.
	code, v = doReq(http.MethodPut, base+"/"+strconv.FormatInt(vid, 10), `{"url":"https://c.example"}`)
	if code != http.StatusOK || v.Weight != 0 {
		t.Fatalf("expected 200 preserving weight 0, got %d weight %d", code, v.Weight)
	}
	// Explicit null resets to the default weight.
	code, v = doReq(http.MethodPut, base+"/"+strconv.FormatInt(vid, 10), `{"weight":null}`)
	if code != http.StatusOK || v.Weight != 1 {
		t.Fatalf("expected 200 with weight reset to 1, got %d weight %d", code, v.Weight)
	}
	// Negative weight is rejected.
	if code, _ := doReq(http.MethodPut, base+"/"+strconv.FormatInt(vid, 10), `{"weight":-2}`); code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative weight, got %d", code)
	}
}

func TestVariantPickSaturates(t *testing.T) {
	// Adversarial weights near MaxInt64 must not overflow the total.
	huge := int64(1) << 62
	list := []variants.Variant{
		{ID: 1, Weight: int(huge)},
		{ID: 2, Weight: int(huge)},
	}
	for i := int64(0); i < 100; i++ {
		if got := variants.Pick(list, i); got == nil {
			t.Fatal("Pick must not return nil for positive weights")
		}
	}
}

func TestLinkUpdateDomainIDNullClears(t *testing.T) {
	repo := links.NewMemoryRepository()
	dstore := domains.NewMemoryStore()
	mux := http.NewServeMux()
	NewLinkAPI(repo).WithDomains(dstore).Routes(mux)

	d, err := dstore.Create(1, "example.test")
	if err != nil {
		t.Fatal(err)
	}
	domainPayload := `"domain_id":` + strconv.FormatInt(d.ID, 10)
	link := createTestLink(t, mux, `{"url":"https://example.com","slug":"domtest",`+domainPayload+`}`)
	if link.DomainID == nil || *link.DomainID != d.ID {
		t.Fatalf("expected domain set, got %+v", link.DomainID)
	}

	put := func(body string) links.Link {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/links/"+strconv.FormatInt(link.ID, 10), strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var updated links.Link
		_ = json.NewDecoder(rec.Body).Decode(&updated)
		return updated
	}

	// Omitted domain_id preserves the domain.
	if updated := put(`{"slug":"domtest"}`); updated.DomainID == nil {
		t.Fatal("omitted domain_id must preserve the domain")
	}
	// Explicit null clears it.
	if updated := put(`{"domain_id":null}`); updated.DomainID != nil {
		t.Fatal("explicit null domain_id must clear the domain")
	}
}

func TestVariantAPIInvalidatesListCache(t *testing.T) {
	repo := links.NewMemoryRepository()
	vstore := variants.NewMemoryStore()
	lc := variants.NewListCache()
	mux := http.NewServeMux()
	NewLinkAPI(repo).Routes(mux)
	NewVariantAPI(repo, vstore).WithListCache(lc).Routes(mux)

	link := createTestLink(t, mux, `{"url":"https://example.com","slug":"cacheinv"}`)
	vstore.AddLink(link.ID)
	base := "/api/v1/links/" + strconv.FormatInt(link.ID, 10) + "/variants"

	// Seed a stale cache entry (as if a redirect had cached "no variants").
	lc.Set(link.ID, nil)

	post := func(body string) int {
		req := httptest.NewRequest(http.MethodPost, base, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := post(`{"url":"https://b.example","weight":2}`); code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", code)
	}
	// Creating a variant must invalidate the cached list.
	if _, ok := lc.Get(link.ID); ok {
		t.Fatal("variant create must invalidate the list cache")
	}
}
