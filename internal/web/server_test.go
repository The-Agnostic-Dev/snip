package web

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/claretnnamocha/snip/internal/store"
)

var lagos = time.FixedZone("WAT", 3600)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Seed(ctx, time.Now(), lagos); err != nil {
		t.Fatal(err)
	}
	h, err := New(st, Config{
		BaseURL:  "https://snip.test",
		Location: lagos,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

var noFollow = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

func get(t *testing.T, rawURL string) (*http.Response, string) {
	t.Helper()
	res, err := noFollow.Get(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	return res, string(body)
}

func TestDashboardListsNewestLinks(t *testing.T) {
	srv := newTestServer(t)
	res, body := get(t, srv.URL+"/")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if !strings.Contains(body, `data-alias="flash-deal"`) || strings.Contains(body, `data-alias="summer-sale"`) {
		t.Error("first page should show the newest links only")
	}
	if !strings.Contains(body, "Showing 1 to 10 of 24") {
		t.Error("missing paging summary")
	}
}

func TestDashboardSearch(t *testing.T) {
	srv := newTestServer(t)
	_, body := get(t, srv.URL+"/?q=mamaput")
	if strings.Count(body, "data-alias=") != 2 {
		t.Errorf("expected 2 results for mamaput, body:\n%s", body)
	}
}

func TestDashboardCreatedAndExpiresColumns(t *testing.T) {
	srv := newTestServer(t)
	_, body := get(t, srv.URL+"/")

	row := func(alias string) string {
		t.Helper()
		i := strings.Index(body, `data-alias="`+alias+`"`)
		if i < 0 {
			t.Fatalf("no row for %s", alias)
		}
		rest := body[i:]
		if j := strings.Index(rest[1:], "data-alias="); j >= 0 {
			rest = rest[:j+1]
		}
		return rest
	}
	cell := func(row, class string) string {
		t.Helper()
		i := strings.Index(row, `class="`+class+`"`)
		if i < 0 {
			t.Fatalf("no %s cell in row: %s", class, row)
		}
		rest := row[i:]
		start := strings.Index(rest, ">") + 1
		end := start + strings.Index(rest[start:], "<")
		return strings.TrimSpace(rest[start:end])
	}

	flash := row("flash-deal") // expires in 2 days
	if got := cell(flash, "expires"); got == "" || got == "Never" {
		t.Errorf("flash-deal expires cell = %q, want the expiry date", got)
	}
	if got := cell(flash, "created"); got == "" || got == "Never" {
		t.Errorf("flash-deal created cell = %q, want the creation date", got)
	}

	news := row("newsletter") // never expires
	if got := cell(news, "expires"); got != "Never" {
		t.Errorf("newsletter expires cell = %q, want Never", got)
	}
	if got := cell(news, "created"); got == "" || got == "Never" {
		t.Errorf("newsletter created cell = %q, want the creation date", got)
	}
}

func TestRedirect(t *testing.T) {
	srv := newTestServer(t)
	res, _ := get(t, srv.URL+"/menu")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://mamaput.example.ng/menu.pdf" {
		t.Fatalf("got %d %q", res.StatusCode, res.Header.Get("Location"))
	}

	res, _ = get(t, srv.URL+"/does-not-exist")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown alias: status %d", res.StatusCode)
	}

	res, _ = get(t, srv.URL+"/webinar")
	if res.StatusCode != http.StatusGone {
		t.Errorf("expired link: status %d", res.StatusCode)
	}
}

func TestCreateLink(t *testing.T) {
	srv := newTestServer(t)
	form := url.Values{"target": {"https://example.com/landing"}, "alias": {"spring-promo"}}
	res, err := noFollow.PostForm(srv.URL+"/links", form)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(res.Header.Get("Location"), "/links/") {
		t.Fatalf("create: %d %q", res.StatusCode, res.Header.Get("Location"))
	}

	res, _ = get(t, srv.URL+"/spring-promo")
	if res.Header.Get("Location") != "https://example.com/landing" {
		t.Errorf("redirect to %q", res.Header.Get("Location"))
	}

	res, err = noFollow.PostForm(srv.URL+"/links", form)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(string(body), "already taken") {
		t.Errorf("duplicate alias: %d", res.StatusCode)
	}
}

func TestCreateLinkValidation(t *testing.T) {
	srv := newTestServer(t)
	res, err := noFollow.PostForm(srv.URL+"/links", url.Values{"target": {""}, "alias": {"a"}})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", res.StatusCode)
	}
	for _, want := range []string{"Enter the address", "Use 3 to 40"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestShowLinkWithClicks(t *testing.T) {
	srv := newTestServer(t)
	res, body := get(t, srv.URL+"/links/3") // "menu"
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if !strings.Contains(body, `data-testid="total-clicks">64<`) {
		t.Error("expected 64 total clicks")
	}
	if strings.Count(body, "data-day=") != 14 {
		t.Error("expected 14 days of stats")
	}
}

func TestUnknownLinkPage(t *testing.T) {
	srv := newTestServer(t)
	for _, path := range []string{"/links/999", "/links/abc"} {
		if res, _ := get(t, srv.URL+path); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d", path, res.StatusCode)
		}
	}
}
