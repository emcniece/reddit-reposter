# Reddit Reposter

A [Devvit](https://developers.reddit.com) app that crossposts Reddit posts from the subreddit it's installed on to a destination subreddit, filtered by flair and/or title.

Flair assigned or changed after a post is created is handled automatically via the `PostFlairUpdate` trigger. Crossposts are deleted if the source post's flair later changes away from the target flair.

## Prerequisites

- [Devvit CLI](https://developers.reddit.com/docs/devvit_cli): `npm install -g devvit`
- Moderator access on both the source and destination subreddits

## Setup

```sh
npm install
devvit login
devvit upload          # publishes the app to Reddit's developer platform
```

Then install the app on your source subreddit:

```sh
devvit install <source-subreddit>
```

After installation, configure the app settings in the subreddit's mod tools under **Installed Apps**:

| Setting | Required | Description |
|---|---|---|
| Destination subreddit | Yes | Where to crosspost (without `r/`) |
| Flair filter | No | Only crosspost posts with this exact flair (case-insensitive). Leave blank to crosspost all posts. |
| Title regex | No | Only crosspost posts whose title matches this JavaScript RegExp. Leave blank to match all titles. |

## How it works

1. **`PostCreate`** — when a post is created with matching flair (or when there is no flair filter), it is crossposted immediately.
2. **`PostFlairUpdate`** — when flair is added or changed:
   - If it now matches the filter and hasn't been crossposted: crosspost it.
   - If it no longer matches and was previously crossposted: remove the crosspost.

Post IDs are tracked in Redis (`xpost:{sourcePostId} → crosspostId`) to prevent duplicates and enable deletion.

## Development

```sh
devvit playtest <source-subreddit>   # live test on a real subreddit
npm run build                         # compile only
```
