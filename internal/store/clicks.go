package store

import (
	"context"
	"fmt"
	"time"
)

// RecordClick stores one visit of a short link.
func (s *Store) RecordClick(ctx context.Context, linkID int64, at time.Time, referrer string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO clicks (link_id, clicked_at, referrer) VALUES (?, ?, ?)`,
		linkID, formatTime(at), referrer); err != nil {
		return fmt.Errorf("insert click: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE links SET click_count = click_count + 1 WHERE id = ?`, linkID); err != nil {
		return fmt.Errorf("bump click count: %w", err)
	}
	return tx.Commit()
}

// ResetClicks deletes the recorded visits of a link.
func (s *Store) ResetClicks(ctx context.Context, linkID int64) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM clicks WHERE link_id = ?`, linkID); err != nil {
		return fmt.Errorf("reset clicks: %w", err)
	}
	return nil
}

// Totals summarises the visits of a link.
type Totals struct {
	Clicks    int
	LastClick time.Time
}

// LinkTotals returns the number of visits and the time of the most recent one.
func (s *Store) LinkTotals(ctx context.Context, linkID int64) (Totals, error) {
	var (
		t    Totals
		last string
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), MAX(clicked_at) FROM clicks WHERE link_id = ?`, linkID).Scan(&t.Clicks, &last)
	if err != nil {
		return Totals{}, fmt.Errorf("link totals: %w", err)
	}
	if t.LastClick, err = parseTime(last); err != nil {
		return Totals{}, err
	}
	return t, nil
}

// DailyClicks counts visits per day since the given instant, keyed by
// YYYY-MM-DD. Days without visits are not included.
func (s *Store) DailyClicks(ctx context.Context, linkID int64, since time.Time) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT substr(clicked_at, 1, 10) AS day, COUNT(*)
		   FROM clicks
		  WHERE link_id = ? AND clicked_at >= ?
		  GROUP BY day`,
		linkID, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("daily clicks: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var (
			day string
			n   int
		)
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		counts[day] = n
	}
	return counts, rows.Err()
}
