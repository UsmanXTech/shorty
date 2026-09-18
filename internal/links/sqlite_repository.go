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

func (r *SQLiteRepository) Create(link Link) (Link, error) {
	_, err := r.db.Exec(
		"INSERT INTO links (slug, url, created_at, updated_at, expires_at) VALUES (?, ?, ?, ?, ?)",
		link.Slug, link.URL, link.CreatedAt.UTC().Format(time.RFC3339Nano),
		link.UpdatedAt.UTC().Format(time.RFC3339Nano), formatTime(link.ExpiresAt),
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
	rows, err := r.db.Query("SELECT id, slug, url, created_at, updated_at, expires_at FROM links ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
		"SELECT id, slug, url, created_at, updated_at, expires_at FROM links WHERE id = ?", id,
	))
}

func (r *SQLiteRepository) GetBySlug(slug string) (Link, error) {
	return scanLink(r.db.QueryRow(
		"SELECT id, slug, url, created_at, updated_at, expires_at FROM links WHERE slug = ?", slug,
	))
}

func (r *SQLiteRepository) Update(link Link) (Link, error) {
	result, err := r.db.Exec(
		"UPDATE links SET slug = ?, url = ?, updated_at = ?, expires_at = ? WHERE id = ?",
		link.Slug, link.URL, link.UpdatedAt.UTC().Format(time.RFC3339Nano),
		formatTime(link.ExpiresAt), link.ID,
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

	if err := row.Scan(&link.ID, &link.Slug, &link.URL, &created, &updated, &expires); err != nil {
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
	if expires.Valid && expires.String != "" {
		value, err := time.Parse(time.RFC3339Nano, expires.String)
		if err != nil {
			return Link{}, err
		}
		link.ExpiresAt = &value
	}
	return link, nil
}

func formatTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func isConstraint(err error) bool {
	return err != nil && strings.Contains(err.Error(), "constraint failed")
}
