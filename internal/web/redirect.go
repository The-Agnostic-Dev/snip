package web

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/claretnnamocha/snip/internal/store"
)

// handleRedirect sends visitors of a short link on to its destination.
func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	alias := r.PathValue("alias")

	link, ok := s.cache.get(alias)
	if !ok {
		var err error
		link, err = s.store.GetLinkByAlias(r.Context(), alias)
		if errors.Is(err, store.ErrNotFound) {
			s.notFound(w)
			return
		}
		if err != nil {
			s.serverError(w, err)
			return
		}
		s.cache.put(link)
	}

	now := s.cfg.Now()
	if link.Expired(now, s.cfg.Location) {
		s.message(w, http.StatusGone, "This link has expired", "The owner of this short link set it to stop working.")
		return
	}

	// Recording the click must not slow down or break the redirect.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
	defer cancel()
	if err := s.store.RecordClick(ctx, link.ID, now, r.Referer()); err != nil {
		s.cfg.Logger.Warn("record click", "alias", alias, "error", err)
	}

	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, link.Target, http.StatusFound)
}
