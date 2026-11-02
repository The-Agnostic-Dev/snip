package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Link is a short link.
type Link struct {
	ID     int64
	Alias  string
	Target string
	// ExpiresOn is the last day (YYYY-MM-DD, in the app's timezone) the link
	// works. Empty means it never expires.
	ExpiresOn  string
	ClickCount int64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Expired reports whether the link has expired at the given instant.
func (l Link) Expired(now time.Time, loc *time.Location) bool {
	if l.ExpiresOn == "" {
		return false
	}
	today := now.In(loc).Format(time.DateOnly)
	return l.ExpiresOn <= today
}

// LinkInput holds the editable fields of a link.
type LinkInput struct {
	Alias     string
	Target    string
	ExpiresOn string
}

const linkColumns = `id, alias, target, COALESCE(expires_on, ''), click_count, created_at, updated_at`

type scanner interface {
	Scan(dest ...any) error
}

func scanLink(row scanner) (Link, error) {
	var (
		l                Link
		created, updated string
	)
	if err := row.Scan(&l.ID, &l.Alias, &l.Target, &l.ExpiresOn, &l.ClickCount, &created, &updated); err != nil {
		return Link{}, err
	}
	var err error
	if l.CreatedAt, err = parseTime(created); err != nil {
		return Link{}, fmt.Errorf("parse created_at: %w", err)
	}
	if l.UpdatedAt, err = parseTime(updated); err != nil {
		return Link{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return l, nil
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// CreateLink inserts a new link.
func (s *Store) CreateLink(ctx context.Context, in LinkInput, now time.Time) (Link, error) {
	ts := formatTime(now)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO links (alias, target, expires_on, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		in.Alias, in.Target, nullable(in.ExpiresOn), ts, ts)
	if err != nil {
		return Link{}, fmt.Errorf("insert link: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Link{}, err
	}
	return s.GetLink(ctx, id)
}

// AliasExists reports whether a link already uses alias.
func (s *Store) AliasExists(ctx context.Context, alias string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM links WHERE alias = ?`, alias).Scan(&n)
	return n > 0, err
}

// GetLink loads a link by id.
func (s *Store) GetLink(ctx context.Context, id int64) (Link, error) {
	l, err := scanLink(s.db.QueryRowContext(ctx, `SELECT `+linkColumns+` FROM links WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

// GetLinkByAlias loads a link by its alias.
func (s *Store) GetLinkByAlias(ctx context.Context, alias string) (Link, error) {
	l, err := scanLink(s.db.QueryRowContext(ctx,
		`SELECT `+linkColumns+` FROM links WHERE alias = ? ORDER BY id LIMIT 1`, alias))
	if errors.Is(err, sql.ErrNoRows) {
		return Link{}, ErrNotFound
	}
	return l, err
}

// UpdateLink changes the destination and expiry of a link. The alias is fixed.
func (s *Store) UpdateLink(ctx context.Context, id int64, in LinkInput, now time.Time) (Link, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE links SET target = ?, expires_on = ?, updated_at = ? WHERE id = ?`,
		in.Target, nullable(in.ExpiresOn), formatTime(now), id)
	if err != nil {
		return Link{}, fmt.Errorf("update link: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Link{}, ErrNotFound
	}
	return s.GetLink(ctx, id)
}

// DeleteLink removes a link and its clicks.
func (s *Store) DeleteLink(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM links WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete link: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListParams filters and pages ListLinks.
type ListParams struct {
	Query  string
	Limit  int
	Offset int
}

func likePattern(q string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(q) + "%"
}

// ListLinks returns links, newest first, matching the query on alias or
// destination, plus the total number of matches.
func (s *Store) ListLinks(ctx context.Context, p ListParams) ([]Link, int, error) {
	where := ""
	var args []any
	if q := strings.TrimSpace(p.Query); q != "" {
		where = `WHERE alias LIKE ? ESCAPE '\' OR target LIKE ? ESCAPE '\'`
		pattern := likePattern(q)
		args = append(args, pattern, pattern)
	}

	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM links `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count links: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+linkColumns+` FROM links `+where+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		append(args, p.Limit, p.Offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list links: %w", err)
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		l, err := scanLink(rows)
		if err != nil {
			return nil, 0, err
		}
		links = append(links, l)
	}
	return links, total, rows.Err()
}
