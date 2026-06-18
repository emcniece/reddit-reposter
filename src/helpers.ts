import type { TriggerContext } from '@devvit/public-api';

export const KV_PREFIX = 'xpost:';
export const MONITOR_KEY = 'monitor:tracked';
export const MONITOR_JOB = 'monitor_check';
export const MONITOR_TTL_MS = 24 * 60 * 60 * 1000;

export function redisKey(sourcePostId: string): string {
  return `${KV_PREFIX}${sourcePostId}`;
}

export function matchesFilters(
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

export async function getSettings(context: TriggerContext): Promise<{
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
