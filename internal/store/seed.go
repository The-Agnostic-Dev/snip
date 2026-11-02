package store

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

type seedLink struct {
	alias       string
	target      string
	createdDays int // days before today
	expiresDays int // days after today, 0 = never, negative = already expired
	clicks      int
}

var seedLinks = []seedLink{
	{"summer-sale", "https://shop.example.ng/collections/summer-sale?utm_source=snip", 58, 0, 140},
	{"black-friday-2025", "https://shop.example.ng/black-friday", 55, -20, 310},
	{"menu", "https://mamaput.example.ng/menu.pdf", 52, 0, 64},
	{"careers", "https://kola.example.ng/careers", 50, 0, 22},
	{"ankara-drop", "https://shop.example.ng/collections/ankara", 47, 0, 88},
	{"webinar", "https://events.example.ng/webinars/fintech-in-lagos", 45, -3, 51},
	{"shoes", "https://shop.example.ng/collections/shoes", 42, 0, 37},
	{"lagos-meetup", "https://events.example.ng/meetups/lagos-devs", 40, 25, 0},
	{"bags", "https://shop.example.ng/collections/bags", 38, 0, 19},
	{"pitch-deck", "https://docs.example.ng/kola/pitch-deck-q3", 36, 0, 9},
	{"wholesale", "https://shop.example.ng/wholesale", 34, 0, 12},
	{"gift-cards", "https://shop.example.ng/gift-cards", 31, 60, 45},
	{"survey", "https://forms.example.ng/customer-survey", 29, 10, 73},
	{"kids", "https://shop.example.ng/collections/kids", 27, 0, 16},
	{"new-arrivals", "https://shop.example.ng/collections/new-arrivals", 24, 0, 102},
	{"returns", "https://shop.example.ng/pages/returns-policy", 21, 0, 8},
	{"podcast", "https://podcasts.example.ng/kola-talks/episode-12", 18, 0, 27},
	{"store-locator", "https://shop.example.ng/pages/stores", 15, 0, 14},
	{"jollof-promo", "https://mamaput.example.ng/promos/jollof-friday", 12, 30, 58},
	{"lookbook", "https://shop.example.ng/lookbook/harmattan", 9, 0, 31},
	{"app", "https://apps.example.ng/kola", 7, 0, 40},
	{"accessories", "https://shop.example.ng/collections/accessories", 5, 45, 11},
	{"newsletter", "https://kola.example.ng/newsletter/october", 3, 0, 6},
	{"flash-deal", "https://shop.example.ng/deals/flash", 1, 2, 4},
}

// Seed fills an empty database with demo links and clicks. Dates are relative
// to now so the demo always looks current. It does nothing if links exist.
func (s *Store) Seed(ctx context.Context, now time.Time, loc *time.Location) error {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM links`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	rng := rand.New(rand.NewPCG(2026, 11))

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, sl := range seedLinks {
		created := today.AddDate(0, 0, -sl.createdDays).Add(9*time.Hour + time.Duration(rng.IntN(480))*time.Minute)
		var expires any
		if sl.expiresDays != 0 {
			expires = today.AddDate(0, 0, sl.expiresDays).Format(time.DateOnly)
		}

		var clicks []time.Time
		if sl.alias == "lagos-meetup" {
			// Reminder messages go out just after midnight, then a lunchtime post.
			for d := 1; d <= 7; d++ {
				day := today.AddDate(0, 0, -d)
				clicks = append(clicks, day.Add(20*time.Minute), day.Add(40*time.Minute), day.Add(12*time.Hour))
			}
		} else {
			// Clicks fall within the last 30 days, and before the expiry date for expired links.
			newest := max(0, -sl.expiresDays)
			oldest := min(sl.createdDays, 30)
			for range sl.clicks {
				daysAgo := newest + rng.IntN(oldest-newest+1)
				at := today.AddDate(0, 0, -daysAgo).Add(time.Duration(6*60+rng.IntN(16*60)) * time.Minute)
				if at.After(now) {
					at = now.Add(-time.Duration(rng.IntN(60)+1) * time.Minute)
				}
				clicks = append(clicks, at)
			}
		}

		res, err := tx.ExecContext(ctx,
			`INSERT INTO links (alias, target, expires_on, click_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			sl.alias, sl.target, expires, len(clicks), formatTime(created), formatTime(created))
		if err != nil {
			return fmt.Errorf("seed link %s: %w", sl.alias, err)
		}
		id, _ := res.LastInsertId()
		for _, at := range clicks {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO clicks (link_id, clicked_at, referrer) VALUES (?, ?, '')`, id, formatTime(at)); err != nil {
				return fmt.Errorf("seed clicks: %w", err)
			}
		}
	}
	return tx.Commit()
}
