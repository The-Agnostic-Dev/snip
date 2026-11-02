# Snip

Snip is a small, self-hosted link shortener. Create short links with your own alias (or a random
one), give them an optional expiry date, and see how many people clicked them each day. Built for
marketing and community teams who want branded short links without a third party service.

## Features

- Custom aliases (`/summer-sale`) or random 6 character aliases
- Optional expiry date: the link works until the end of that day, then shows an "expired" page
- Public redirect at `/{alias}` that records each click
- Dashboard with search (alias or destination) and pagination
- Per-link stats: total clicks, last click and clicks per day for the last 14 days
- Edit the destination or expiry of a link, reset its stats, delete it

## Stack

- Go 1.24, standard library only for HTTP (`net/http` with method and wildcard patterns) and
  templates (`html/template`)
- SQLite through the pure Go driver `modernc.org/sqlite` (no CGO)
- Templates and static files are embedded in the binary

## Running

```bash
go run ./cmd/snip          # or: make run
```

Open http://localhost:5110. The app starts with an in-memory database seeded with demo links and
clicks (relative to today), so there is something to look at straight away.

With Docker:

```bash
docker compose up --build
```

### Configuration

| Variable        | Default                  | Description                                        |
| --------------- | ------------------------ | -------------------------------------------------- |
| `PORT`          | `5110`                   | HTTP port                                          |
| `SNIP_BASE_URL` | `http://localhost:$PORT` | Origin shown in short links and copied by "Copy"   |
| `SNIP_TIMEZONE` | `Africa/Lagos`           | Timezone for expiry dates and daily stats          |
| `SNIP_DB`       | `:memory:`               | SQLite file path, or `:memory:`                    |
| `SNIP_SEED`     | `true`                   | Seed demo data when the database is empty          |

## Tests

```bash
go test ./...              # or: make test
```

The tests use in-memory SQLite and `httptest`; nothing needs to be running.

## Layout

```
cmd/snip/              main: configuration, database, HTTP server
internal/store/        SQLite schema, links, clicks, demo seed
internal/shortlink/    alias, destination and expiry rules, random aliases
internal/web/          handlers, redirect cache, templates and static assets
  templates/           layout, dashboard, form, link detail, message pages
  static/              app.css, app.js (copy buttons, confirmations)
```

## Routes

| Method | Path                    | What it does                          |
| ------ | ----------------------- | ------------------------------------- |
| GET    | `/`                     | Dashboard (`q`, `page`)               |
| GET    | `/links/new`            | New link form                         |
| POST   | `/links`                | Create link                           |
| GET    | `/links/{id}`           | Link stats                            |
| GET    | `/links/{id}/edit`      | Edit form                             |
| POST   | `/links/{id}`           | Update destination and expiry         |
| POST   | `/links/{id}/delete`    | Delete link                           |
| POST   | `/links/{id}/reset`     | Reset click statistics                |
| GET    | `/{alias}`              | Public redirect (302, 404 or 410)     |
| GET    | `/healthz`              | Health check                          |

## Notes

- Times are stored in UTC (`2006-01-02T15:04:05Z` text). Expiry dates are plain calendar dates in
  `SNIP_TIMEZONE`.
- Redirect lookups are cached in memory per process.
