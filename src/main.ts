import { Devvit, SettingScope, TriggerContext } from '@devvit/public-api';

Devvit.configure({ redditAPI: true, redis: true });

// ---------------------------------------------------------------------------
// Settings (configured by the mod at installation time)
// ---------------------------------------------------------------------------

Devvit.addSettings([
  {
    type: 'group',
    label: 'Source mode — crosspost from this subreddit',
    helpText:
      'Use this mode when installing on the subreddit you want to crosspost FROM. ' +
      'Set a destination subreddit to activate source mode. ' +
      'Posts that match the flair and title filters will be automatically crossposted.',
    fields: [
      {
        name: 'destination_subreddit',
        label: 'Destination subreddit (without r/)',
        helpText: 'Where matching posts will be crossposted. Leave blank if this is a monitor installation.',
        type: 'string',
        scope: SettingScope.Installation,
        isSecret: false,
      },
      {
        name: 'flair_filter',
        label: 'Flair to match (leave blank to match all flairs)',
        helpText: 'Case-insensitive. Only posts with this exact flair are crossposted. Posts flaired after creation are caught automatically.',
        type: 'string',
        scope: SettingScope.Installation,
        isSecret: false,
      },
      {
        name: 'exclude_flair',
        label: 'Flair to exclude (leave blank to exclude nothing)',
        helpText: 'Case-insensitive. Posts with this flair are never crossposted. Also used in monitor mode — set the same value on both installations.',
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
    ],
  },
  {
    type: 'group',
    label: 'Monitor mode — auto-remove crossposts on this subreddit',
    helpText:
      'Use this mode when installing on the subreddit you want to crosspost TO. ' +
      'Leave "Destination subreddit" blank and set "Flair to exclude" to the same value as the source installation. ' +
      'The app will check source post flairs every 5 minutes and delete any crosspost whose source gains the excluded flair.',
    fields: [],
  },
]);

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const KV_PREFIX = 'xpost:';
// Sorted set used in monitor mode: member = "crosspostId|sourcePostId", score = timestamp (ms)
const MONITOR_KEY = 'monitor:tracked';
const MONITOR_JOB = 'monitor_check';
// Expire monitor entries after 24 hours (posts are unlikely to be flair-changed after that)
const MONITOR_TTL_MS = 24 * 60 * 60 * 1000;

function redisKey(sourcePostId: string): string {
  return `${KV_PREFIX}${sourcePostId}`;
}

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
// AppInstall — schedule the monitor cron job for all installations.
// The job checks at runtime whether this installation is in monitor mode and
// returns early if not, so scheduling it unconditionally is safe.
// (Settings are not yet configured at install time, so we can't filter here.)
// ---------------------------------------------------------------------------

Devvit.addTrigger({
  event: 'AppInstall',
  async onEvent(_event, context) {
    const jobs = await context.scheduler.listJobs();
    if (!jobs.some(j => j.name === MONITOR_JOB)) {
      await context.scheduler.runJob({ name: MONITOR_JOB, cron: '*/5 * * * *' });
      console.log(`scheduled ${MONITOR_JOB} cron job`);
    }
  },
});

// ---------------------------------------------------------------------------
// PostCreate — source mode: crosspost matching new posts.
//              monitor mode: track incoming crossposts for flair polling.
// ---------------------------------------------------------------------------

Devvit.addTrigger({
  event: 'PostCreate',
  async onEvent(event, context) {
    const { destination, flairFilter, excludeFlair, titleRegex } = await getSettings(context);

    if (destination) {
      // SOURCE MODE: crosspost if the new post matches all filters.
      const postId = event.post?.id;
      const title = event.post?.title ?? '';
      const flair = event.post?.linkFlair?.text ?? '';

      if (!postId) return;

      // If a positive flair filter is set but the post has no flair yet, wait —
      // PostFlairUpdate will fire when flair is assigned.
      if (flairFilter && !flair) return;

      if (!matchesFilters(title, flair, flairFilter, excludeFlair, titleRegex)) return;

      const existing = await context.redis.get(redisKey(postId));
      if (existing) return;

      const post = await context.reddit.getPostById(postId);
      const crosspost = await post.crosspost({ subredditName: destination, title: post.title });
      await context.redis.set(redisKey(postId), crosspost.id);
      console.log(`crossposted ${postId} ("${title}") → r/${destination} as ${crosspost.id}`);

    } else if (excludeFlair) {
      // MONITOR MODE: if the new post is a crosspost, track it for periodic flair checks.
      const postId = event.post?.id;
      if (!postId) return;

      const post = await context.reddit.getPostById(postId);
      const sourcePostId = post.crosspostParentId;
      if (!sourcePostId) return;

      await context.redis.zAdd(MONITOR_KEY, { score: Date.now(), member: `${postId}|${sourcePostId}` });
      console.log(`monitor: tracking crosspost ${postId} (source: ${sourcePostId})`);
    }
  },
});

// ---------------------------------------------------------------------------
// PostFlairUpdate — source mode only.
// Crossposts when flair matches; deletes the crosspost when flair changes away.
// ---------------------------------------------------------------------------

Devvit.addTrigger({
  event: 'PostFlairUpdate',
  async onEvent(event, context) {
    const { destination, flairFilter, excludeFlair, titleRegex } = await getSettings(context);
    if (!destination) return;

    if (!flairFilter && !excludeFlair) return;

    const postId = event.post?.id;
    const title = event.post?.title ?? '';
    const flair = event.post?.linkFlair?.text ?? '';

    if (!postId) return;

    const existingCrosspostId = await context.redis.get(redisKey(postId));
    const matches = matchesFilters(title, flair, flairFilter, excludeFlair, titleRegex);

    if (matches && !existingCrosspostId) {
      const post = await context.reddit.getPostById(postId);
      const crosspost = await post.crosspost({ subredditName: destination, title: post.title });
      await context.redis.set(redisKey(postId), crosspost.id);
      console.log(`crossposted ${postId} ("${title}") → r/${destination} as ${crosspost.id} (via flair update)`);
    } else if (!matches && existingCrosspostId) {
      try {
        const crosspost = await context.reddit.getPostById(existingCrosspostId);
        await crosspost.delete();
        await context.redis.del(redisKey(postId));
        console.log(`deleted crosspost ${existingCrosspostId} (source ${postId} flair changed to "${flair}")`);
      } catch (err) {
        await context.redis.del(redisKey(postId));
        console.log(
          `could not delete crosspost ${existingCrosspostId} in r/${destination} ` +
          `(source post ${postId} flair changed to "${flair}"). ` +
          `Install the app on r/${destination} in monitor mode to handle this automatically. Error: ${err}`,
        );
      }
    }
  },
});

// ---------------------------------------------------------------------------
// Scheduler job — monitor mode only.
// Runs every 5 minutes, fetches each tracked source post's flair, and
// deletes the local crosspost if the source has gained the excluded flair.
// Also expires entries older than MONITOR_TTL_MS to keep the queue bounded.
// ---------------------------------------------------------------------------

Devvit.addSchedulerJob({
  name: MONITOR_JOB,
  async onRun(_event, context) {
    const { excludeFlair, destination } = await getSettings(context as TriggerContext);

    // Only operate in monitor mode (installed on destination sub, exclude_flair set).
    if (destination || !excludeFlair) return;

    // Expire entries older than MONITOR_TTL_MS to bound queue size.
    const cutoff = Date.now() - MONITOR_TTL_MS;
    await context.redis.zRemRangeByScore(MONITOR_KEY, 0, cutoff);

    const members = await context.redis.zRange(MONITOR_KEY, 0, -1);
    if (!members.length) return;

    const toRemove: string[] = [];

    for (const { member } of members) {
      const pipeIdx = member.indexOf('|');
      if (pipeIdx === -1) continue;
      const crosspostId = member.slice(0, pipeIdx);
      const sourcePostId = member.slice(pipeIdx + 1);

      try {
        const sourcePost = await context.reddit.getPostById(sourcePostId);
        const sourceFlair = sourcePost.flair?.text ?? '';

        if (sourceFlair.toLowerCase() === excludeFlair.toLowerCase()) {
          const crosspost = await context.reddit.getPostById(crosspostId);
          await crosspost.delete();
          toRemove.push(member);
          console.log(
            `monitor: deleted crosspost ${crosspostId} ` +
            `(source ${sourcePostId} has excluded flair "${sourceFlair}")`,
          );
        }
      } catch (err) {
        console.error(`monitor: error checking ${crosspostId}: ${err}`);
      }
    }

    if (toRemove.length > 0) {
      await context.redis.zRem(MONITOR_KEY, toRemove);
    }
  },
});

// ---------------------------------------------------------------------------
// Seed feature — mod menu item that backfills recent posts to the destination.
// Only visible in source mode (destination_subreddit is set).
// ---------------------------------------------------------------------------

const seedForm = Devvit.createForm(
  {
    title: 'Seed destination subreddit',
    description: 'Crossposts recent matching posts that have not been crossposted yet.',
    fields: [
      {
        type: 'number',
        name: 'count',
        label: 'Number of recent posts to check',
        helpText: 'Posts that pass your flair/title filters will be crossposted. Max 50.',
        defaultValue: 25,
      },
    ],
    acceptLabel: 'Seed',
  },
  async (event, context) => {
    const count = Math.min(Math.max(1, Math.round((event.values.count as number) ?? 25)), 50);
    const { destination, flairFilter, excludeFlair, titleRegex } = await getSettings(
      context as unknown as TriggerContext,
    );

    if (!destination) {
      context.ui.showToast('No destination subreddit configured.');
      return;
    }

    const subredditName = await context.reddit.getCurrentSubredditName();
    const posts = await context.reddit.getNewPosts({ subredditName, limit: count }).get(count);

    let crossposted = 0;
    let skipped = 0;

    for (const post of posts) {
      const flair = post.flair?.text ?? '';
      if (flairFilter && !flair) { skipped++; continue; }
      if (!matchesFilters(post.title, flair, flairFilter, excludeFlair, titleRegex)) { skipped++; continue; }

      const existing = await context.redis.get(redisKey(post.id));
      if (existing) { skipped++; continue; }

      try {
        const crosspost = await post.crosspost({ subredditName: destination, title: post.title });
        await context.redis.set(redisKey(post.id), crosspost.id);
        crossposted++;
        console.log(`seed: crossposted ${post.id} ("${post.title}") → r/${destination} as ${crosspost.id}`);
      } catch (err) {
        console.error(`seed: failed to crosspost ${post.id}: ${err}`);
        skipped++;
      }
    }

    context.ui.showToast(
      `Seeded ${crossposted} post${crossposted !== 1 ? 's' : ''} to r/${destination}` +
      (skipped > 0 ? ` (${skipped} skipped)` : '') +
      '.',
    );
  },
);

Devvit.addMenuItem({
  label: 'Seed destination subreddit',
  location: 'subreddit',
  forUserType: 'moderator',
  async onPress(_event, context) {
    const { destination } = await getSettings(context as unknown as TriggerContext);
    if (!destination) {
      context.ui.showToast('Not in source mode — set a destination subreddit first.');
      return;
    }
    context.ui.showForm(seedForm);
  },
});

export default Devvit;
