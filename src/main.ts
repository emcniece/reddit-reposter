import { Devvit, SettingScope, TriggerContext } from '@devvit/public-api';

Devvit.configure({ redditAPI: true, redis: true });

// ---------------------------------------------------------------------------
// Settings (configured by the mod at installation time)
// ---------------------------------------------------------------------------

Devvit.addSettings([
  {
    name: 'destination_subreddit',
    label: 'Destination subreddit (without r/)',
    helpText: 'Posts that match the filters below will be crossposted here.',
    type: 'string',
    scope: SettingScope.Installation,
    isSecret: false,
  },
  {
    name: 'flair_filter',
    label: 'Flair to match (leave blank to match all flairs)',
    helpText: 'Case-insensitive. When set, only posts with this exact flair are crossposted. Posts flaired after creation are caught automatically.',
    type: 'string',
    scope: SettingScope.Installation,
    isSecret: false,
  },
  {
    name: 'exclude_flair',
    label: 'Flair to exclude (leave blank to exclude nothing)',
    helpText: 'Case-insensitive. Posts with this flair are never crossposted. If a crossposted post is later given this flair, the crosspost is removed.',
    type: 'string',
    scope: SettingScope.Installation,
    isSecret: false,
  },
  {
    name: 'title_regex',
    label: 'Title regex filter (leave blank to match all titles)',
    helpText: 'JavaScript RegExp syntax. Example: ^\\[OC\\]',
    type: 'string',
    scope: SettingScope.Installation,
    isSecret: false,
  },
]);

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const KV_PREFIX = 'xpost:';

function redisKey(sourcePostId: string): string {
  return `${KV_PREFIX}${sourcePostId}`;
}

/**
 * Returns true if the post's title and flair pass all configured filters.
 * Empty filter values are ignored (match everything).
 */
function matchesFilters(
  title: string,
  flair: string,
  flairFilter: string,
  excludeFlair: string,
  titleRegex: string,
): boolean {
  if (flairFilter && flair.toLowerCase() !== flairFilter.toLowerCase()) return false;
  if (excludeFlair && flair.toLowerCase() === excludeFlair.toLowerCase()) return false;
  if (titleRegex) {
    try {
      if (!new RegExp(titleRegex).test(title)) return false;
    } catch {
      console.error(`Invalid title_regex setting: ${titleRegex}`);
    }
  }
  return true;
}

async function getSettings(context: TriggerContext): Promise<{
  destination: string;
  flairFilter: string;
  excludeFlair: string;
  titleRegex: string;
}> {
  const [destination, flairFilter, excludeFlair, titleRegex] = await Promise.all([
    context.settings.get<string>('destination_subreddit'),
    context.settings.get<string>('flair_filter'),
    context.settings.get<string>('exclude_flair'),
    context.settings.get<string>('title_regex'),
  ]);
  return {
    destination: destination ?? '',
    flairFilter: flairFilter ?? '',
    excludeFlair: excludeFlair ?? '',
    titleRegex: titleRegex ?? '',
  };
}

// ---------------------------------------------------------------------------
// PostCreate — handles posts that already have flair at creation time,
// and posts with no flair filter configured.
// ---------------------------------------------------------------------------

Devvit.addTrigger({
  event: 'PostCreate',
  async onEvent(event, context) {
    const { destination, flairFilter, excludeFlair, titleRegex } = await getSettings(context);
    if (!destination) return;

    const postId = event.post?.id;
    const title = event.post?.title ?? '';
    const flair = event.post?.linkFlair?.text ?? '';

    if (!postId) return;

    // If a positive flair filter is set but the post has no flair yet, wait —
    // PostFlairUpdate will fire when flair is assigned.
    // (An exclude-only filter does not need to wait: no flair = not excluded.)
    if (flairFilter && !flair) return;

    if (!matchesFilters(title, flair, flairFilter, excludeFlair, titleRegex)) return;

    // Dedup: skip if we've already crossposted this post.
    const existing = await context.redis.get(redisKey(postId));
    if (existing) return;

    const post = await context.reddit.getPostById(postId);
    const crosspost = await post.crosspost({ subredditName: destination, title: post.title });
    await context.redis.set(redisKey(postId), crosspost.id);

    console.log(`crossposted ${postId} ("${title}") → r/${destination} as ${crosspost.id}`);
  },
});

// ---------------------------------------------------------------------------
// PostFlairUpdate — handles flair being set or changed after post creation.
// Crossposts when flair matches; deletes the crosspost when flair no longer matches.
// ---------------------------------------------------------------------------

Devvit.addTrigger({
  event: 'PostFlairUpdate',
  async onEvent(event, context) {
    const { destination, flairFilter, excludeFlair, titleRegex } = await getSettings(context);
    if (!destination) return;

    // Flair change events are only relevant when at least one flair filter is set.
    if (!flairFilter && !excludeFlair) return;

    const postId = event.post?.id;
    const title = event.post?.title ?? '';
    const flair = event.post?.linkFlair?.text ?? '';

    if (!postId) return;

    const existingCrosspostId = await context.redis.get(redisKey(postId));
    const matches = matchesFilters(title, flair, flairFilter, excludeFlair, titleRegex);

    if (matches && !existingCrosspostId) {
      // Flair now matches and we haven't crossposted yet — do it now.
      const post = await context.reddit.getPostById(postId);
      const crosspost = await post.crosspost({ subredditName: destination, title: post.title });
      await context.redis.set(redisKey(postId), crosspost.id);
      console.log(`crossposted ${postId} ("${title}") → r/${destination} as ${crosspost.id} (via flair update)`);
    } else if (!matches && existingCrosspostId) {
      // Flair changed away from the target — remove the crosspost.
      const crosspost = await context.reddit.getPostById(existingCrosspostId);
      await crosspost.remove(false);
      await context.redis.del(redisKey(postId));
      console.log(`removed crosspost ${existingCrosspostId} (source ${postId} flair changed to "${flair}")`);
    }
  },
});

export default Devvit;
