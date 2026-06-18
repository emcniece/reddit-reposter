# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./cmd/reposter

# Run once (cron mode)
REDDIT_CLIENT_ID=... REDDIT_CLIENT_SECRET=... REDDIT_USERNAME=... REDDIT_PASSWORD=... \
  ./reposter --config config.yaml

# Run as daemon
./reposter --config config.yaml --daemon --interval 5m

# Test
go test ./...

# Lint/vet
go vet ./...

# Docker
docker build -t reddit-reposter .
docker run --rm \
  -e REDDIT_CLIENT_ID=... -e REDDIT_CLIENT_SECRET=... \
  -e REDDIT_USERNAME=... -e REDDIT_PASSWORD=... \
  -v $(pwd)/config.yaml:/config.yaml \
  reddit-reposter
```

## Architecture

```
cmd/reposter/main.go       Entry point. Parses --config, --daemon, --interval flags.
                           Single-run by default (exits after one pass); --daemon loops.
internal/config/           Loads config.yaml (routes + filters) and binds Reddit OAuth
                           credentials from env vars. Compiles title_regex at load time.
internal/reddit/           Thin wrapper around go-reddit/v2. Exposes NewPosts, Crosspost,
                           IsDuplicate. Crosspost submits a link post to the original
                           permalink (go-reddit has no native crosspost endpoint).
internal/routes/           Iterates configured routes, filters posts, deduplicates via
                           Reddit's /duplicates endpoint, and crossposts matches.
```

## Configuration

Routes live in `config.yaml` (committed, no secrets). Credentials are env vars only:

| Env var | Purpose |
|---|---|
| `REDDIT_CLIENT_ID` | OAuth app client ID |
| `REDDIT_CLIENT_SECRET` | OAuth app client secret |
| `REDDIT_USERNAME` | Bot account username |
| `REDDIT_PASSWORD` | Bot account password |

See `config.yaml.example` for route syntax.

## GitHub Actions

| Workflow | Trigger | Effect |
|---|---|---|
| `ci.yml` | Every push/PR | `go vet`, `go test`, `go build` |
| `release.yml` | Push to `main` or version tag | Builds multi-arch image, pushes to `ghcr.io/<owner>/reddit-reposter` |
| `cron.yml` | Every 15 min (+ manual) | Pulls `:latest` image, mounts `config.yaml`, runs once |

Required GitHub secrets for cron: `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`, `REDDIT_USERNAME`, `REDDIT_PASSWORD`.

## Flair handling

Flair can be set or changed after a post is created, so the fetching strategy differs by filter type:

- **Flair filter configured:** Uses Reddit's search API (`flair:"X"` query, `sort=new`) instead of `/new`. This catches posts whose flair was applied or changed after they were first published — each run re-queries and finds newly-flaired posts naturally.
- **No flair filter:** Falls back to fetching `/new` posts directly.

**Stale flair cleanup:** After crossposting, the processor scans the bot's recent posts in the destination sub and fetches the current flair of each original post via `GET /by_id/t3_{id}` (a raw Reddit API call, since `go-reddit`'s `Post` struct does not expose `link_flair_text`). If flair no longer matches the route's filter, the crosspost is deleted.

## Known Limitations

- **Crosspost vs link post.** Reddit's native crosspost API is not exposed by `go-reddit`. The workaround submits a link post to `https://www.reddit.com<permalink>`, which Reddit renders similarly but is technically a link post.
- **Flair search pagination.** `SearchByFlair` fetches one page (25 posts). Subreddits with very high posting volume may miss posts between runs. Increase `defaultFetchLimit` in `internal/routes/processor.go` if needed.
