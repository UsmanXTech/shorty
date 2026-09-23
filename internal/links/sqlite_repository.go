package links

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

const linkColumns = "id, slug, url, created_at, updated_at, expires_at, max_clicks, clicks, password_hash, team_id, domain_id"

func (r *SQLiteRepository) Create(link Link) (Link, error) {
	_, err := r.db.Exec(
		"INSERT INTO links (slug, url, created_at, updated_at, expires_at, max_clicks, password_hash, team_id, domain_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		link.Slug, link.URL, link.CreatedAt.UTC().Format(time.RFC3339Nano),
		link.UpdatedAt.UTC().Format(time.RFC3339Nano), formatTime(link.ExpiresAt), link.MaxClicks,
		nullString(link.PasswordHash), teamOrDefault(link.TeamID), nullInt64(link.DomainID),
	)
	if err != nil {
		if isConstraint(err) {
			return Link{}, ErrConflict
		}
		return Link{}, err
	}
	return r.GetBySlug(link.Slug)
}

func (r *SQLiteRepository) List() ([]Link, error) {
	return r.queryLinks("SELECT " + linkColumns + " FROM links ORDER BY id DESC")
}

func (r *SQLiteRepository) ListByTeam(teamID int64) ([]Link, error) {
	rows, err := r.db.Query("SELECT "+linkColumns+" FROM links WHERE team_id = ? ORDER BY id DESC", teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAllLinks(rows)
}

func (r *SQLiteRepository) queryLinks(query string, args ...any) ([]Link, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAllLinks(rows)
}

func scanAllLinks(rows *sql.Rows) ([]Link, error) {
	var result []Link
	for rows.Next() {
		link, err := scanLink(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, link)
	}
	return result, rows.Err()
}

func (r *SQLiteRepository) GetByID(id int64) (Link, error) {
	return scanLink(r.db.QueryRow(
		"SELECT "+linkColumns+" FROM links WHERE id = ?", id,
	))
}

func (r *SQLiteRepository) GetBySlug(slug string) (Link, error) {
	return scanLink(r.db.QueryRow(
		"SELECT "+linkColumns+" FROM links WHERE slug = ?", slug,
	))
}

func (r *SQLiteRepository) GetBySlugAndDomain(slug string, domainID *int64) (Link, error) {
	if domainID == nil {
		return scanLink(r.db.QueryRow(
			"SELECT "+linkColumns+" FROM links WHERE slug = ? AND domain_id IS NULL", slug,
		))
	}
	return scanLink(r.db.QueryRow(
		"SELECT "+linkColumns+" FROM links WHERE slug = ? AND domain_id = ?", slug, *domainID,
	))
}

func (r *SQLiteRepository) Update(link Link) (Link, error) {
	result, err := r.db.Exec(
		"UPDATE links SET slug = ?, url = ?, updated_at = ?, expires_at = ?, max_clicks = ?, password_hash = ?, team_id = ?, domain_id = ? WHERE id = ?",
		link.Slug, link.URL, link.UpdatedAt.UTC().Format(time.RFC3339Nano),
		formatTime(link.ExpiresAt), link.MaxClicks, nullString(link.PasswordHash),
		teamOrDefault(link.TeamID), nullInt64(link.DomainID), link.ID,
	)
	if err != nil {
		if isConstraint(err) {
			return Link{}, ErrConflict
		}
		return Link{}, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Link{}, err
	}
	if n == 0 {
		return Link{}, ErrNotFound
	}
	return r.GetByID(link.ID)
}

func (r *SQLiteRepository) Delete(id int64) error {
	result, err := r.db.Exec("DELETE FROM links WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanLink(row scanner) (Link, error) {
	var link Link
	var created, updated string
	var expires sql.NullString
	var maxClicks sql.NullInt64
	var clicks int64
	var passwordHash sql.NullString
	var teamID sql.NullInt64
	var domainID sql.NullInt64

	if err := row.Scan(&link.ID, &link.Slug, &link.URL, &created, &updated, &expires, &maxClicks, &clicks, &passwordHash, &teamID, &domainID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Link{}, ErrNotFound
		}
		return Link{}, err
	}

	var err error
	link.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Link{}, err
	}
	link.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return Link{}, err
	}
	if maxClicks.Valid {
		value := maxClicks.Int64
		link.MaxClicks = &value
	}
	link.Clicks = clicks
	if passwordHash.Valid {
		link.PasswordHash = passwordHash.String
	}
	if teamID.Valid {
		link.TeamID = teamID.Int64
	} else {
		link.TeamID = 1
	}
	if domainID.Valid {
		value := domainID.Int64
		link.DomainID = &value
	}
	if expires.Valid && expires.String != "" {
		value, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return Link{}, err
		}
		link.ExpiresAt = &value
	}
	return link, nil
}

func (r *SQLiteRepository) IncrementClicks(slug string) (Link, error) {
	// Single round trip: UPDATE ... RETURNING both increments and fetches
	// the row. The fallback SELECT below only runs when nothing was
	// updated (missing slug or exhausted click limit).
	link, err := scanLink(r.db.QueryRow(
		`UPDATE links SET clicks = clicks + 1 WHERE slug = ? AND (max_clicks IS NULL OR clicks < max_clicks) RETURNING `+linkColumns,
		slug,
	))
	if err == nil {
		return link, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Link{}, err
	}
	// No row was updated: either the link doesn't exist or its click
	// limit was already reached. Distinguish the two so callers can
	// report the right status.
	var clicks int64
	var maxClicks sql.NullInt64
	qerr := r.db.QueryRow(`SELECT clicks, max_clicks FROM links WHERE slug = ?`, slug).Scan(&clicks, &maxClicks)
	if errors.Is(qerr, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	if qerr != nil {
		return Link{}, qerr
	}
	if maxClicks.Valid && clicks >= maxClicks.Int64 {
		return Link{}, ErrClickLimit
	}
	return Link{}, ErrNotFound
}

func formatTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt64(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func teamOrDefault(teamID int64) int64 {
	if teamID <= 0 {
		return 1
	}
	return teamID
}

func isConstraint(err error) bool {
	return err != nil && strings.Contains(err.Error(), "constraint failed")
}
