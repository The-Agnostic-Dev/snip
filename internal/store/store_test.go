package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

var lagos = time.FixedZone("WAT", 3600)

func openTest(t *testing.T) *Store {
	t.Helper()
	st, err := Open(context.Background(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestLinkLifecycle(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

	l, err := st.CreateLink(ctx, LinkInput{Alias: "promo", Target: "https://example.com/a"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if l.ID == 0 || l.Alias != "promo" || l.ExpiresOn != "" || !l.CreatedAt.Equal(now) {
		t.Fatalf("unexpected link %+v", l)
	}

	got, err := st.GetLinkByAlias(ctx, "promo")
	if err != nil || got.ID != l.ID {
		t.Fatalf("GetLinkByAlias = %+v, %v", got, err)
	}

	updated, err := st.UpdateLink(ctx, l.ID, LinkInput{Target: "https://example.com/b", ExpiresOn: "2026-04-01"}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if updated.Target != "https://example.com/b" || updated.ExpiresOn != "2026-04-01" || updated.Alias != "promo" {
		t.Fatalf("unexpected update %+v", updated)
	}

	if err := st.DeleteLink(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetLink(ctx, l.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListLinksSearchAndPaging(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	base := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	targets := []string{"https://shop.example.ng/a", "https://blog.example.ng/b", "https://shop.example.ng/c", "https://docs.example.ng/d"}
	for i, target := range targets {
		alias := []string{"alpha", "bravo", "charlie", "delta"}[i]
		if _, err := st.CreateLink(ctx, LinkInput{Alias: alias, Target: target}, base.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	links, total, err := st.ListLinks(ctx, ListParams{Query: "shop", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(links) != 2 || links[0].Alias != "charlie" || links[1].Alias != "alpha" {
		t.Fatalf("search shop = %d %+v", total, links)
	}

	links, total, err = st.ListLinks(ctx, ListParams{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if total != 4 || len(links) != 2 || links[0].Alias != "delta" {
		t.Fatalf("first page = %d %+v", total, links)
	}

	links, _, _ = st.ListLinks(ctx, ListParams{Query: "100%", Limit: 10})
	if len(links) != 0 {
		t.Fatalf("wildcards should be escaped, got %+v", links)
	}
}

func TestClicks(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	l, _ := st.CreateLink(ctx, LinkInput{Alias: "promo", Target: "https://example.com"}, now)

	for _, at := range []time.Time{now.Add(-48 * time.Hour), now.Add(-time.Hour), now} {
		if err := st.RecordClick(ctx, l.ID, at, "https://ig.example"); err != nil {
			t.Fatal(err)
		}
	}

	got, _ := st.GetLink(ctx, l.ID)
	if got.ClickCount != 3 {
		t.Errorf("ClickCount = %d, want 3", got.ClickCount)
	}

	totals, err := st.LinkTotals(ctx, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if totals.Clicks != 3 || !totals.LastClick.Equal(now) {
		t.Errorf("totals = %+v", totals)
	}

	daily, err := st.DailyClicks(ctx, l.ID, now.Add(-72*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if daily["2026-03-08"] != 1 || daily["2026-03-10"] != 2 || len(daily) != 2 {
		t.Errorf("daily = %v", daily)
	}
}

func TestExpired(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		expires string
		want    bool
	}{
		{"", false},
		{"2026-03-09", true},
		{"2026-01-31", true},
		{"2026-03-11", false},
		{"2027-01-01", false},
	}
	for _, tt := range tests {
		if got := (Link{ExpiresOn: tt.expires}).Expired(now, lagos); got != tt.want {
			t.Errorf("Expired(%q) = %v, want %v", tt.expires, got, tt.want)
		}
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := openTest(t)
	now := time.Now()
	for range 2 {
		if err := st.Seed(ctx, now, lagos); err != nil {
			t.Fatal(err)
		}
	}
	_, total, err := st.ListLinks(ctx, ListParams{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if total != len(seedLinks) {
		t.Errorf("total = %d, want %d", total, len(seedLinks))
	}
}
