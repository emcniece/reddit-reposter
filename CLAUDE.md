# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```sh
npm install                           # install dependencies
npm run build                         # compile (devvit build)
devvit login                          # authenticate with Reddit
devvit upload                         # publish app to Reddit's developer platform
devvit install <subreddit>            # install on a subreddit
devvit playtest <subreddit>           # live test (hot-reload) on a real subreddit
node_modules/.bin/tsc --noEmit        # type-check without building
```

## Architecture

This is a [Devvit](https://developers.reddit.com) app — Reddit hosts and executes it, no self-hosting required.

```
src/main.ts      All app logic: settings declaration, trigger handlers, helpers.
devvit.yaml      App name and version.
package.json     Dependencies (@devvit/public-api) and build scripts.
```

**Dual-mode app**: the same codebase handles two installation roles:

- **Source mode** (default): installed on the source subreddit. `destination_subreddit` is set. Crossposts matching posts to the destination.
- **Monitor mode**: installed on the destination subreddit. `destination_subreddit` is empty, `exclude_flair` is set. Tracks incoming crossposts and deletes them if their source post's flair changes to the excluded flair.

**One installation = one source→destination route.** Settings (destination sub, flair filter, title regex, exclude flair) are configured per-installation in Reddit's mod tools.

## Triggers and jobs

| Event | Source mode | Monitor mode |
|---|---|---|
| `AppInstall` | Schedules `monitor_check` cron | Schedules `monitor_check` cron |
| `PostCreate` | Crossposts if filters match | Tracks crosspost→source mapping in Redis |
| `PostFlairUpdate` | Crossposts or deletes crosspost based on new flair | — (not used) |
| `monitor_check` (cron, every 5 min) | No-op (returns early) | Expires entries >24h old; checks source flair; deletes crosspost if excluded |
| "Seed destination subreddit" (menu item) | Backfills recent posts to destination | — (not used) |

## State

Redis (via `context.redis`) tracks crossposted posts:

**Source mode:**
- `xpost:{sourcePostId}` → crosspost post ID (dedup key, enables deletion)

**Monitor mode:**
- `monitor:tracked` (sorted set): member = `crosspostId|sourcePostId`, score = timestamp

## Flair handling

- Flair text comparison is **case-insensitive**.
- `PostFlairUpdate` fires with the new flair already set on `event.post.linkFlair.text` — no extra fetch needed for the flair value.
- `context.reddit.getPostById()` is only called when actually crossposting or deleting (to get the rich `Post` model with `.crosspost()` and `.delete()` methods).
- `post.crosspostParentId` — `T3ID | undefined`; present only on crossposted posts.

## Key SDK types

- `event.post?.linkFlair?.text` — flair text in trigger event payloads (from proto `PostV2`)
- `post.flair?.text` — flair text on the rich `Post` model returned by `getPostById()`
- `post.crosspost({ subredditName, title })` — title is required by `CrosspostOptions`
- `post.delete()` — author-level delete; restricted to the installed subreddit (cross-sub delete fails)
- `context.redis.get/set/del` — Redis KV
- `context.redis.zAdd/zRange/zRem` — Redis sorted set
- `context.scheduler.listJobs()` / `runJob({ name, cron })` — scheduler; `scheduler` does NOT need to be declared in `Devvit.configure()`
- `TriggerContext` and `JobContext` (from `types/scheduler.d.ts`) are structurally identical; `TriggerContext` can be used for both
