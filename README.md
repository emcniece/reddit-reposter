# Reddit Reposter

A [Devvit](https://developers.reddit.com) app that automatically crossposts Reddit posts from one subreddit to another, with filtering by flair and title. Flair changes are tracked in real-time, and a companion monitor installation on the destination subreddit handles automatic crosspost removal when source posts gain an excluded flair.

## Features

- **Automatic crossposting** — new posts are crossposted immediately on creation, or when flair is assigned if a flair filter is set
- **Real-time flair tracking** — `PostFlairUpdate` triggers crosspost or removal as flair changes
- **Flair inclusion filter** — only crosspost posts with a specific flair
- **Flair exclusion filter** — never crosspost posts with a specific flair; remove the crosspost if the source post gains it later
- **Title regex filter** — only crosspost posts whose title matches a JavaScript regex
- **Monitor mode** — a second installation on the destination subreddit polls source post flairs every 5 minutes and deletes stale crossposts (solves Devvit's cross-subreddit action restriction)
- **Seed tool** — mod menu item to backfill recent posts to the destination on demand

## Prerequisites

- [Devvit CLI](https://developers.reddit.com/docs/devvit_cli): `npm install -g devvit`
- Moderator access on both the source and destination subreddits

## Setup

```sh
npm install
devvit login
devvit upload          # publishes the app to Reddit's developer platform
```

### Source installation (required)

Install on the subreddit you want to crosspost **from**:

```sh
devvit install <source-subreddit>
```

Configure in the subreddit's mod tools under **Installed Apps**:

| Setting | Required | Description |
|---|---|---|
| Destination subreddit | Yes | Where to crosspost (without `r/`) |
| Flair to match | No | Only crosspost posts with this exact flair (case-insensitive). Leave blank to match all posts. |
| Flair to exclude | No | Never crosspost posts with this flair. Crossposts are removed if the source gains it later. |
| Title regex | No | Only crosspost posts whose title matches this JavaScript RegExp (e.g. `^\[OC\]`). |

### Monitor installation (optional, recommended)

Devvit cannot delete posts outside the subreddit the app is installed on. To automatically remove crossposts when a source post gains the excluded flair, install the app a second time on the **destination** subreddit:

```sh
devvit install <destination-subreddit>
# or:
# devvit playtest sub_reposter_dev

devvit logs LangfordStagingSub --show-timestamps
devvit logs sub_reposter_dev --show-timestamps
```

Configure with:
- **Destination subreddit**: leave blank
- **Flair to exclude**: same value as the source installation

The monitor checks source post flairs every 5 minutes and deletes any crosspost whose source has gained the excluded flair. Entries are automatically expired after 24 hours.

## Seeding existing posts

After installation, use the **Seed destination subreddit** mod menu item (visible in the source subreddit's menu) to backfill recent posts. It checks up to 50 recent posts, applies your configured filters, and crossposts any that haven't been crossposted yet. Posts already tracked in Redis are skipped to prevent duplicates.

## How it works

**Source mode** (`destination_subreddit` is set):

1. **`PostCreate`** — crosspost immediately if the post matches all filters. If a flair filter is set but the post has no flair yet, waits for `PostFlairUpdate`.
2. **`PostFlairUpdate`** — crosspost if flair now matches and the post hasn't been crossposted; delete the crosspost if flair no longer matches.

**Monitor mode** (`destination_subreddit` empty, `exclude_flair` set):

1. **`PostCreate`** — when a new post arrives in the destination sub and it has a `crosspostParentId`, record the `crosspostId|sourcePostId` pair in a Redis sorted set.
2. **`monitor_check`** (cron, every 5 min) — expire entries older than 24 hours, then for each remaining entry fetch the source post's flair. If it matches the excluded flair, delete the crosspost and remove it from tracking.

Redis keys:
- `xpost:{sourcePostId}` → crosspost post ID (source mode, dedup + deletion)
- `monitor:tracked` sorted set: member = `crosspostId|sourcePostId`, score = timestamp (monitor mode)

## Development

```sh
devvit playtest <subreddit>    # live test with hot reload on a real subreddit
npm run build                   # compile only
node_modules/.bin/tsc --noEmit  # type-check without building
```
