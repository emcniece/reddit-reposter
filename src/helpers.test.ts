import { describe, it, expect, vi, beforeEach } from 'vitest';
import { matchesFilters, getSettings, redisKey, KV_PREFIX, MONITOR_TTL_MS } from './helpers.js';

// ---------------------------------------------------------------------------
// redisKey
// ---------------------------------------------------------------------------

describe('redisKey', () => {
  it('prefixes the source post ID', () => {
    expect(redisKey('abc123')).toBe(`${KV_PREFIX}abc123`);
  });
});

// ---------------------------------------------------------------------------
// matchesFilters
// ---------------------------------------------------------------------------

describe('matchesFilters', () => {
  describe('no filters set', () => {
    it('matches any post', () => {
      expect(matchesFilters('Any title', 'Any flair', '', '', '')).toBe(true);
    });

    it('matches a post with no flair', () => {
      expect(matchesFilters('Title', '', '', '', '')).toBe(true);
    });
  });

  describe('flairFilter', () => {
    it('matches when flair equals the filter', () => {
      expect(matchesFilters('Title', 'News', 'News', '', '')).toBe(true);
    });

    it('rejects when flair does not match the filter', () => {
      expect(matchesFilters('Title', 'Sports', 'News', '', '')).toBe(false);
    });

    it('is case-insensitive', () => {
      expect(matchesFilters('Title', 'NEWS', 'news', '', '')).toBe(true);
      expect(matchesFilters('Title', 'news', 'NEWS', '', '')).toBe(true);
    });

    it('rejects a post with no flair when filter is set', () => {
      expect(matchesFilters('Title', '', 'News', '', '')).toBe(false);
    });
  });

  describe('excludeFlair', () => {
    it('rejects when flair equals the excluded flair', () => {
      expect(matchesFilters('Title', 'Politics', '', 'Politics', '')).toBe(false);
    });

    it('matches when flair does not equal the excluded flair', () => {
      expect(matchesFilters('Title', 'News', '', 'Politics', '')).toBe(true);
    });

    it('is case-insensitive', () => {
      expect(matchesFilters('Title', 'POLITICS', '', 'politics', '')).toBe(false);
      expect(matchesFilters('Title', 'politics', '', 'POLITICS', '')).toBe(false);
    });

    it('matches a post with no flair when exclude is set', () => {
      expect(matchesFilters('Title', '', '', 'Politics', '')).toBe(true);
    });
  });

  describe('flairFilter and excludeFlair together', () => {
    it('rejects when flair matches flairFilter but also matches excludeFlair', () => {
      // Edge case: same flair set as both include and exclude — exclude wins.
      expect(matchesFilters('Title', 'News', 'News', 'News', '')).toBe(false);
    });

    it('matches when flair satisfies include but not exclude', () => {
      expect(matchesFilters('Title', 'News', 'News', 'Politics', '')).toBe(true);
    });

    it('rejects when flair satisfies neither include nor exclude', () => {
      expect(matchesFilters('Title', 'Sports', 'News', 'Politics', '')).toBe(false);
    });
  });

  describe('titleRegex', () => {
    it('matches when title satisfies the regex', () => {
      expect(matchesFilters('[OC] My photo', '', '', '', '^\\[OC\\]')).toBe(true);
    });

    it('rejects when title does not satisfy the regex', () => {
      expect(matchesFilters('My photo', '', '', '', '^\\[OC\\]')).toBe(false);
    });

    it('combines with flair filters', () => {
      expect(matchesFilters('[OC] Photo', 'Art', 'Art', '', '^\\[OC\\]')).toBe(true);
      expect(matchesFilters('Photo', 'Art', 'Art', '', '^\\[OC\\]')).toBe(false);
      expect(matchesFilters('[OC] Photo', 'Sports', 'Art', '', '^\\[OC\\]')).toBe(false);
    });

    it('logs an error and does not filter on an invalid regex', () => {
      const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
      expect(matchesFilters('Title', '', '', '', '[')).toBe(true);
      expect(spy).toHaveBeenCalledWith(expect.stringContaining('Invalid title_regex'));
      spy.mockRestore();
    });
  });
});

// ---------------------------------------------------------------------------
// getSettings
// ---------------------------------------------------------------------------

describe('getSettings', () => {
  const makeContext = (values: Record<string, string | undefined>) => ({
    settings: {
      get: vi.fn((key: string) => Promise.resolve(values[key])),
    },
  });

  it('returns all settings from context', async () => {
    const ctx = makeContext({
      destination_subreddit: 'DestSub',
      flair_filter: 'News',
      exclude_flair: 'Politics',
      title_regex: '^\\[OC\\]',
    });

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await getSettings(ctx as any);
    expect(result).toEqual({
      destination: 'DestSub',
      flairFilter: 'News',
      excludeFlair: 'Politics',
      titleRegex: '^\\[OC\\]',
      copyFlair: false,
    });
  });

  it('defaults missing settings to empty strings and false', async () => {
    const ctx = makeContext({});
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const result = await getSettings(ctx as any);
    expect(result).toEqual({
      destination: '',
      flairFilter: '',
      excludeFlair: '',
      titleRegex: '',
      copyFlair: false,
    });
  });

  it('reads all five settings in parallel', async () => {
    const get = vi.fn((key: string) => Promise.resolve(key));
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    await getSettings({ settings: { get } } as any);
    expect(get).toHaveBeenCalledTimes(5);
    expect(get).toHaveBeenCalledWith('destination_subreddit');
    expect(get).toHaveBeenCalledWith('flair_filter');
    expect(get).toHaveBeenCalledWith('exclude_flair');
    expect(get).toHaveBeenCalledWith('title_regex');
    expect(get).toHaveBeenCalledWith('copy_flair');
  });
});

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

describe('MONITOR_TTL_MS', () => {
  it('is exactly 24 hours in milliseconds', () => {
    expect(MONITOR_TTL_MS).toBe(24 * 60 * 60 * 1000);
  });
});
