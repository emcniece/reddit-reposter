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

**One installation = one source→destination route.** The app is installed on the source subreddit. Settings (destination sub, flair filter, title regex) are configured per-installation in Reddit's mod tools.

## Triggers

| Trigger | Purpose |
|---|---|
| `PostCreate` | Crossposts immediately if the post already has matching flair at creation time (or if no flair filter is set). If a flair filter is set and the post has no flair yet, does nothing — waits for `PostFlairUpdate`. |
| `PostFlairUpdate` | Handles flair being set or changed after creation. Crossposts when flair starts matching; deletes the crosspost when flair stops matching. No-op if no flair filter is configured. |

## State

Redis (via `context.redis`) tracks crossposted posts:
- Key: `xpost:{sourcePostId}` → Value: crosspost post ID
- Used for dedup on `PostCreate` and for finding the crosspost to delete on `PostFlairUpdate`.

## Flair handling

- Flair text comparison is **case-insensitive**.
- `PostFlairUpdate` fires with the new flair already set on `event.post.linkFlair.text` — no extra fetch needed for the flair value.
- `context.reddit.getPostById()` is only called when actually crossposting or deleting (to get the rich `Post` model with `.crosspost()` and `.remove()` methods).

## Key SDK types

- `event.post?.linkFlair?.text` — flair text in trigger event payloads (from proto `PostV2`)
- `post.flair?.text` — flair text on the rich `Post` model returned by `getPostById()`
- `post.crosspost({ subredditName, title })` — title is required by `CrosspostOptions`
- `post.remove(isSpam?)` — mod-removes the post
- `context.redis.get/set/del` — Redis-backed KV store
