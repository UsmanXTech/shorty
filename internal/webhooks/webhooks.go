package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// retryBackoff is the delay between delivery attempts. Tests shrink it
// so the retry path does not slow the suite.
var retryBackoff = time.Second

var (
	ErrNotFound = errors.New("webhook not found")
	ErrConflict = errors.New("webhook already exists")
)

const (
	EventLinkCreated = "link.created"
	EventLinkClicked = "link.clicked"
	EventLinkExpired = "link.expired"
)

var AllEvents = []string{EventLinkCreated, EventLinkClicked, EventLinkExpired}

func ValidEvent(e string) bool {
	for _, v := range AllEvents {
		if v == e {
			return true
		}
	}
	return false
}

type Webhook struct {
	ID        int64     `json:"id"`
	TeamID    int64     `json:"team_id"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Secret    string    `json:"-"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	// PlaintextSecret is populated only in the creation/rotation response.
	// It is never read from the database and never appears in list/get.
	PlaintextSecret string `json:"secret,omitempty"`
}

type Delivery struct {
	ID          int64     `json:"id"`
	WebhookID   int64     `json:"webhook_id"`
	Event       string    `json:"event"`
	StatusCode  *int      `json:"status_code,omitempty"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	AttemptedAt time.Time `json:"attempted_at"`
}

type Store interface {
	Create(teamID int64, url string, events []string) (Webhook, error)
	List(teamID int64) ([]Webhook, error)
	Get(id int64) (Webhook, error)
	Update(id int64, url string, events []string, active bool) (Webhook, error)
	RotateSecret(id int64) (Webhook, error)
	Delete(id int64) error
	ListActive(teamID int64, event string) ([]Webhook, error)
	RecordDelivery(d Delivery, payload string) error
	ListDeliveries(webhookID int64, limit int) ([]Delivery, error)
}

// NewSecret generates a random webhook signing secret.
func NewSecret() string {
	buf := make([]byte, 24)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Sign computes the HMAC-SHA256 signature header value for a payload.
func Sign(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// extraBlockedNets are non-public ranges that netip.Addr.IsGlobalUnicast does
// not cover (carrier-grade NAT, benchmarking and documentation ranges).
var extraBlockedNets = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("2001:db8::/32"),
}

// isPublicIP reports whether ip is safe to connect to from the server:
// globally routable and not in a reserved/test range.
func isPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return false
	}
	ip = ip.Unmap()
	for _, p := range extraBlockedNets {
		if p.Contains(ip) {
			return false
		}
	}
	return true
}

// ValidateURL checks that a webhook target is an absolute http(s) URL whose
// host is not a private, loopback, link-local or otherwise reserved address.
// Hostnames get a best-effort DNS check; unresolvable names are allowed here
// because the delivery-time dialer is the authoritative enforcement point.
func ValidateURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("url must use http or https")
	}
	if u.User != nil {
		return errors.New("url must not contain credentials")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("url must have a host")
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !isPublicIP(ip) {
			return errors.New("url must not target a private or reserved IP address")
		}
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil // fail open; the safe dialer enforces at delivery time
	}
	for _, ip := range ips {
		if addr, err := netip.ParseAddr(ip.String()); err == nil && isPublicIP(addr) {
			return nil
		}
	}
	return errors.New("url host does not resolve to a public IP address")
}

// resolvePublicIPs resolves host and returns only its public IPs.
func resolvePublicIPs(ctx context.Context, host string) ([]string, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		if !isPublicIP(ip) {
			return nil, errors.New("webhook target is not a public IP")
		}
		return []string{ip.String()}, nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, ip := range ips {
		if addr, err := netip.ParseAddr(ip.String()); err == nil && isPublicIP(addr) {
			out = append(out, addr.String())
		}
	}
	if len(out) == 0 {
		return nil, errors.New("webhook host does not resolve to a public IP")
	}
	return out, nil
}

// dialPublicIP dials only public IPs, closing the DNS-rebinding hole where a
// hostname resolves to a public IP at validation time and a private one later.
func dialPublicIP(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("only TCP dialing is allowed")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := resolvePublicIPs(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	var firstErr error
	for _, ip := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		if err == nil {
			return conn, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return nil, firstErr
}

// dialPublicIPTLS wraps dialPublicIP with a TLS handshake that keeps the
// original hostname for SNI and certificate verification.
func dialPublicIPTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	conn, err := dialPublicIP(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	tlsConn := tls.Client(conn, &tls.Config{ServerName: host})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tlsConn, nil
}

// NewSafeHTTPClient returns an HTTP client that refuses to connect to
// non-public addresses, including on redirects.
func NewSafeHTTPClient() *http.Client {
	transport := &http.Transport{
		DialContext:           dialPublicIP,
		DialTLSContext:        dialPublicIPTLS,
		MaxIdleConns:          16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
	}
	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if err := ValidateURL(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
}

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func encodeEvents(events []string) string {
	data, _ := json.Marshal(events)
	return string(data)
}

func decodeEvents(raw string) []string {
	var events []string
	_ = json.Unmarshal([]byte(raw), &events)
	return events
}

func (s *SQLiteStore) Create(teamID int64, url string, events []string) (Webhook, error) {
	for _, e := range events {
		if !ValidEvent(e) {
			return Webhook{}, fmt.Errorf("unknown event %q", e)
		}
	}
	res, err := s.db.Exec(
		"INSERT INTO webhooks (team_id, url, events, secret, active, created_at) VALUES (?, ?, ?, ?, 1, ?)",
		teamID, url, encodeEvents(events), NewSecret(), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return Webhook{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Webhook{}, err
	}
	return s.Get(id)
}

func (s *SQLiteStore) List(teamID int64) ([]Webhook, error) {
	rows, err := s.db.Query("SELECT id, team_id, url, events, secret, active, created_at FROM webhooks WHERE team_id = ? ORDER BY id", teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Get(id int64) (Webhook, error) {
	return scanWebhook(s.db.QueryRow(
		"SELECT id, team_id, url, events, secret, active, created_at FROM webhooks WHERE id = ?", id,
	))
}

func (s *SQLiteStore) Update(id int64, url string, events []string, active bool) (Webhook, error) {
	for _, e := range events {
		if !ValidEvent(e) {
			return Webhook{}, fmt.Errorf("unknown event %q", e)
		}
	}
	activeInt := 0
	if active {
		activeInt = 1
	}
	res, err := s.db.Exec(
		"UPDATE webhooks SET url = ?, events = ?, active = ? WHERE id = ?",
		url, encodeEvents(events), activeInt, id,
	)
	if err != nil {
		return Webhook{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Webhook{}, ErrNotFound
	}
	return s.Get(id)
}

func (s *SQLiteStore) Delete(id int64) error {
	res, err := s.db.Exec("DELETE FROM webhooks WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RotateSecret generates a fresh signing secret for the webhook.
func (s *SQLiteStore) RotateSecret(id int64) (Webhook, error) {
	res, err := s.db.Exec("UPDATE webhooks SET secret = ? WHERE id = ?", NewSecret(), id)
	if err != nil {
		return Webhook{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Webhook{}, ErrNotFound
	}
	return s.Get(id)
}

func (s *SQLiteStore) ListActive(teamID int64, event string) ([]Webhook, error) {
	rows, err := s.db.Query("SELECT id, team_id, url, events, secret, active, created_at FROM webhooks WHERE team_id = ? AND active = 1", teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Webhook
	for rows.Next() {
		w, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		for _, e := range w.Events {
			if e == event {
				out = append(out, w)
				break
			}
		}
	}
	return out, rows.Err()
}

func (s *SQLiteStore) RecordDelivery(d Delivery, payload string) error {
	var statusCode sql.NullInt64
	if d.StatusCode != nil {
		statusCode = sql.NullInt64{Int64: int64(*d.StatusCode), Valid: true}
	}
	success := 0
	if d.Success {
		success = 1
	}
	_, err := s.db.Exec(
		"INSERT INTO webhook_deliveries (webhook_id, event, payload, status_code, success, error, attempted_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		d.WebhookID, d.Event, payload, statusCode, success, nullString(d.Error),
		d.AttemptedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *SQLiteStore) ListDeliveries(webhookID int64, limit int) ([]Delivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.Query(
		"SELECT id, webhook_id, event, status_code, success, error, attempted_at FROM webhook_deliveries WHERE webhook_id = ? ORDER BY id DESC LIMIT ?",
		webhookID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Delivery
	for rows.Next() {
		var d Delivery
		var statusCode sql.NullInt64
		var errText sql.NullString
		var attempted string
		var success int
		if err := rows.Scan(&d.ID, &d.WebhookID, &d.Event, &statusCode, &success, &errText, &attempted); err != nil {
			return nil, err
		}
		if statusCode.Valid {
			code := int(statusCode.Int64)
			d.StatusCode = &code
		}
		d.Success = success == 1
		if errText.Valid {
			d.Error = errText.String
		}
		t, err := time.Parse(time.RFC3339Nano, attempted)
		if err != nil {
			return nil, err
		}
		d.AttemptedAt = t
		out = append(out, d)
	}
	return out, rows.Err()
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWebhook(row scanner) (Webhook, error) {
	var w Webhook
	var eventsRaw, created string
	var active int
	if err := row.Scan(&w.ID, &w.TeamID, &w.URL, &eventsRaw, &w.Secret, &active, &created); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Webhook{}, ErrNotFound
		}
		return Webhook{}, err
	}
	w.Events = decodeEvents(eventsRaw)
	w.Active = active == 1
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Webhook{}, err
	}
	w.CreatedAt = t
	return w, nil
}

// Dispatcher delivers webhook events asynchronously with retries.
type Dispatcher struct {
	store  Store
	queue  chan dispatchJob
	client *http.Client
	wg     sync.WaitGroup
	once   sync.Once
	closed atomic.Bool
}

type dispatchJob struct {
	hook    Webhook
	event   string
	payload map[string]any
}

func NewDispatcher(store Store, workers int) *Dispatcher {
	return NewDispatcherWithClient(store, workers, NewSafeHTTPClient())
}

// NewDispatcherWithClient is NewDispatcher with an injectable HTTP client
// (used by tests to deliver to local test servers).
func NewDispatcherWithClient(store Store, workers int, client *http.Client) *Dispatcher {
	if workers <= 0 {
		workers = 2
	}
	if client == nil {
		client = NewSafeHTTPClient()
	}
	d := &Dispatcher{
		store:  store,
		queue:  make(chan dispatchJob, 256),
		client: client,
	}
	for i := 0; i < workers; i++ {
		d.wg.Add(1)
		go d.worker()
	}
	return d
}

func (d *Dispatcher) Close() {
	d.once.Do(func() {
		d.closed.Store(true)
		close(d.queue)
	})
	d.wg.Wait()
}

// Emit queues an event for every active webhook of the team subscribed to it.
// Never blocks, and never panics: events emitted after Close are dropped.
func (d *Dispatcher) Emit(teamID int64, event string, payload map[string]any) {
	if d == nil || d.closed.Load() {
		return
	}
	hooks, err := d.store.ListActive(teamID, event)
	if err != nil || len(hooks) == 0 {
		return
	}
	defer func() { _ = recover() }()
	for _, hook := range hooks {
		select {
		case d.queue <- dispatchJob{hook: hook, event: event, payload: payload}:
		default:
		}
	}
}

func (d *Dispatcher) worker() {
	defer d.wg.Done()
	for job := range d.queue {
		d.deliver(job)
	}
}

func (d *Dispatcher) deliver(job dispatchJob) {
	body, err := json.Marshal(map[string]any{
		"event":      job.event,
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"data":       job.payload,
	})
	delivery := Delivery{WebhookID: job.hook.ID, Event: job.event, AttemptedAt: time.Now().UTC()}
	if err != nil {
		delivery.Error = err.Error()
		_ = d.store.RecordDelivery(delivery, "")
		return
	}
	var lastErr string
	var statusCode *int
	ok := false
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(retryBackoff)
		}
		req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, job.hook.URL, bytes.NewReader(body))
		if err != nil {
			lastErr = err.Error()
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "shorty-webhooks/1.0")
		req.Header.Set("X-Shorty-Event", job.event)
		req.Header.Set("X-Shorty-Signature", Sign(job.hook.Secret, body))
		resp, err := d.client.Do(req)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		_ = resp.Body.Close()
		code := resp.StatusCode
		statusCode = &code
		if code >= 200 && code < 300 {
			ok = true
			lastErr = ""
			break
		}
		lastErr = fmt.Sprintf("unexpected status %d", code)
	}
	delivery.Success = ok
	delivery.StatusCode = statusCode
	delivery.Error = strings.TrimSpace(lastErr)
	_ = d.store.RecordDelivery(delivery, string(body))
}
