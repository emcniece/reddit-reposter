# Reddit Reposter

Reposts Reddit posts from one subreddit to another based on title regex and flair filters. Useful for curating a subset of content or surfacing specific posts to a smaller audience.

Posts whose flair is set or changed after creation are handled correctly — the service re-evaluates candidates on every run using Reddit's search API.

## Reddit app setup

1. Go to https://www.reddit.com/prefs/apps and create a new app
2. Choose **script** as the app type
3. Set redirect URI to `http://localhost` (unused but required)
4. Note the **client ID** (under the app name) and **client secret**

## Configuration

Copy the example config and edit it:

```sh
cp config.yaml.example config.yaml
```

```yaml
# config.yaml
routes:
  - source: leagueoflegends        # source subreddit (no r/ prefix)
    destination: my_lol_highlights  # destination subreddit
    filters:
      title_regex: '^\[Highlight\]' # optional; Go regexp syntax
      flair: "Highlight"            # optional; exact match (case-insensitive)
```

Both filters are optional. If neither is set, all new posts from the source are reposted.

## Credentials

Set these environment variables (never put them in `config.yaml`):

```sh
export REDDIT_CLIENT_ID=your_client_id
export REDDIT_CLIENT_SECRET=your_client_secret
export REDDIT_USERNAME=your_bot_account
export REDDIT_PASSWORD=your_bot_password
```

## Running

**Run once** (suitable for cron):
```sh
go run ./cmd/reposter --config config.yaml
```

**Run as a daemon** (polls on an interval):
```sh
go run ./cmd/reposter --config config.yaml --daemon --interval 10m
```

**Docker:**
```sh
docker build -t reddit-reposter .
docker run --rm \
  -e REDDIT_CLIENT_ID -e REDDIT_CLIENT_SECRET \
  -e REDDIT_USERNAME -e REDDIT_PASSWORD \
  -v $(pwd)/config.yaml:/config.yaml \
  reddit-reposter
```

## GitHub Actions deployment

1. Push the repo to GitHub
2. Add the four Reddit credential secrets in **Settings → Secrets and variables → Actions**
3. Push to `main` — the `release` workflow builds and pushes the image to `ghcr.io/<you>/reddit-reposter:latest`
4. The `cron` workflow then runs automatically every 15 minutes using that image

Trigger a manual run anytime via **Actions → Cron Repost → Run workflow**.
