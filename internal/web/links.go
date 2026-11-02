package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/claretnnamocha/snip/internal/shortlink"
	"github.com/claretnnamocha/snip/internal/store"
)

const perPage = 10

type linkRow struct {
	store.Link
	Expired bool
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	links, total, err := s.store.ListLinks(r.Context(), store.ListParams{
		Query:  q,
		Limit:  perPage,
		Offset: (page - 1) * perPage,
	})
	if err != nil {
		s.serverError(w, err)
		return
	}

	now := s.cfg.Now()
	rows := make([]linkRow, len(links))
	for i, l := range links {
		rows[i] = linkRow{Link: l, Expired: l.Expired(now, s.cfg.Location)}
	}

	first, last := 0, 0
	if len(rows) > 0 {
		first = (page-1)*perPage + 1
		last = first + len(rows) - 1
	}
	s.render(w, http.StatusOK, "dashboard", map[string]any{
		"Title":    "Your links",
		"Links":    rows,
		"Query":    q,
		"Total":    total,
		"First":    first,
		"Last":     last,
		"Page":     page,
		"PrevPage": page - 1,
		"NextPage": page + 1,
		"HasPrev":  page > 1,
		"HasNext":  page*perPage < total,
	})
}

func (s *Server) handleNewLink(w http.ResponseWriter, _ *http.Request) {
	s.renderForm(w, http.StatusOK, nil, shortlink.Form{}, nil)
}

func (s *Server) renderForm(w http.ResponseWriter, status int, link *store.Link, form shortlink.Form, errs shortlink.Errors) {
	title := "New short link"
	if link != nil {
		title = "Edit " + link.Alias
	}
	s.render(w, status, "form", map[string]any{
		"Title":  title,
		"Link":   link,
		"Form":   form,
		"Errors": errs,
		"Today":  s.cfg.Now().In(s.cfg.Location).Format(time.DateOnly),
	})
}

func readForm(r *http.Request) shortlink.Form {
	return shortlink.Form{
		Target:    r.PostFormValue("target"),
		Alias:     r.PostFormValue("alias"),
		ExpiresOn: r.PostFormValue("expires_on"),
	}.Normalize()
}

func (s *Server) handleCreateLink(w http.ResponseWriter, r *http.Request) {
	form := readForm(r)
	now := s.cfg.Now()
	errs := shortlink.Validate(form, true, now, s.cfg.Location)
	if len(errs) > 0 {
		s.renderForm(w, http.StatusUnprocessableEntity, nil, form, errs)
		return
	}

	alias := form.Alias
	if alias == "" {
		generated, err := s.uniqueAlias(r)
		if err != nil {
			s.serverError(w, err)
			return
		}
		alias = generated
	} else {
		taken, err := s.store.AliasExists(r.Context(), alias)
		if err != nil {
			s.serverError(w, err)
			return
		}
		if taken {
			s.renderForm(w, http.StatusUnprocessableEntity, nil, form, shortlink.Errors{"alias": "That alias is already taken"})
			return
		}
	}

	link, err := s.store.CreateLink(r.Context(), store.LinkInput{
		Alias:     alias,
		Target:    form.Target,
		ExpiresOn: form.ExpiresOn,
	}, now)
	if err != nil {
		s.serverError(w, err)
		return
	}
	s.setFlash("success", fmt.Sprintf("Short link %s created", s.shortURL(link.Alias)))
	http.Redirect(w, r, fmt.Sprintf("/links/%d", link.ID), http.StatusSeeOther)
}

func (s *Server) uniqueAlias(r *http.Request) (string, error) {
	for range 10 {
		alias := shortlink.RandomAlias(6)
		taken, err := s.store.AliasExists(r.Context(), alias)
		if err != nil {
			return "", err
		}
		if !taken {
			return alias, nil
		}
	}
	return "", errors.New("could not generate a free alias")
}

// loadLink reads the {id} path value and loads the link, writing a 404 if needed.
func (s *Server) loadLink(w http.ResponseWriter, r *http.Request) (store.Link, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		s.notFound(w)
		return store.Link{}, false
	}
	link, err := s.store.GetLink(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		s.notFound(w)
		return store.Link{}, false
	}
	if err != nil {
		s.serverError(w, err)
		return store.Link{}, false
	}
	return link, true
}

type dayCount struct {
	Day     string
	Clicks  int
	Percent int
}

func (s *Server) handleShowLink(w http.ResponseWriter, r *http.Request) {
	link, ok := s.loadLink(w, r)
	if !ok {
		return
	}

	totals, err := s.store.LinkTotals(r.Context(), link.ID)
	if err != nil {
		s.serverError(w, err)
		return
	}

	const days = 14
	now := s.cfg.Now().In(s.cfg.Location)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.cfg.Location)
	start := today.AddDate(0, 0, -(days - 1))
	counts, err := s.store.DailyClicks(r.Context(), link.ID, start)
	if err != nil {
		s.serverError(w, err)
		return
	}

	series := make([]dayCount, days)
	peak := 1
	for i := range series {
		day := start.AddDate(0, 0, i).Format(time.DateOnly)
		series[i] = dayCount{Day: day, Clicks: counts[day]}
		peak = max(peak, counts[day])
	}
	for i := range series {
		series[i].Percent = series[i].Clicks * 100 / peak
	}

	s.render(w, http.StatusOK, "show", map[string]any{
		"Title":   link.Alias,
		"Link":    link,
		"Expired": link.Expired(s.cfg.Now(), s.cfg.Location),
		"Totals":  totals,
		"Daily":   series,
	})
}

func (s *Server) handleEditLink(w http.ResponseWriter, r *http.Request) {
	link, ok := s.loadLink(w, r)
	if !ok {
		return
	}
	s.renderForm(w, http.StatusOK, &link, shortlink.Form{Target: link.Target, Alias: link.Alias, ExpiresOn: link.ExpiresOn}, nil)
}

func (s *Server) handleUpdateLink(w http.ResponseWriter, r *http.Request) {
	link, ok := s.loadLink(w, r)
	if !ok {
		return
	}
	form := readForm(r)
	form.Alias = link.Alias
	errs := shortlink.Validate(form, false, s.cfg.Now(), s.cfg.Location)
	if len(errs) > 0 {
		s.renderForm(w, http.StatusUnprocessableEntity, &link, form, errs)
		return
	}

	if _, err := s.store.UpdateLink(r.Context(), link.ID, store.LinkInput{
		Target:    form.Target,
		ExpiresOn: form.ExpiresOn,
	}, s.cfg.Now()); err != nil {
		s.serverError(w, err)
		return
	}
	s.setFlash("success", "Link updated")
	http.Redirect(w, r, fmt.Sprintf("/links/%d", link.ID), http.StatusSeeOther)
}

func (s *Server) handleDeleteLink(w http.ResponseWriter, r *http.Request) {
	link, ok := s.loadLink(w, r)
	if !ok {
		return
	}
	if err := s.store.DeleteLink(r.Context(), link.ID); err != nil {
		s.serverError(w, err)
		return
	}
	s.cache.forget(link.Alias)
	s.setFlash("success", fmt.Sprintf("Deleted %s", s.shortURL(link.Alias)))
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleResetStats(w http.ResponseWriter, r *http.Request) {
	link, ok := s.loadLink(w, r)
	if !ok {
		return
	}
	if err := s.store.ResetClicks(r.Context(), link.ID); err != nil {
		s.serverError(w, err)
		return
	}
	s.setFlash("success", "Statistics reset")
	http.Redirect(w, r, fmt.Sprintf("/links/%d", link.ID), http.StatusSeeOther)
}
