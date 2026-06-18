package routes

import (
	"context"
	"errors"
	"testing"

	"github.com/emcniece/reddit-reposter/internal/config"
	"github.com/emcniece/reddit-reposter/internal/reddit"
)

// mockClient implements redditClient for testing.
type mockClient struct {
	newPosts      func(ctx context.Context, subreddit string, limit int) ([]*reddit.Post, error)
	searchByFlair func(ctx context.Context, subreddit, flair string, limit int) ([]*reddit.Post, error)
	isDuplicate   func(ctx context.Context, post *reddit.Post, dest string) (bool, error)
	crosspost     func(ctx context.Context, post *reddit.Post, dest string) error
	botPostsInSub func(ctx context.Context, subreddit string) ([]*reddit.Post, error)
	fetchFlair    func(ctx context.Context, postID string) (string, error)
	deletePost    func(ctx context.Context, fullID string) error
}

func (m *mockClient) NewPosts(ctx context.Context, subreddit string, limit int) ([]*reddit.Post, error) {
	return m.newPosts(ctx, subreddit, limit)
}
func (m *mockClient) SearchByFlair(ctx context.Context, subreddit, flair string, limit int) ([]*reddit.Post, error) {
	return m.searchByFlair(ctx, subreddit, flair, limit)
}
func (m *mockClient) IsDuplicate(ctx context.Context, post *reddit.Post, dest string) (bool, error) {
	return m.isDuplicate(ctx, post, dest)
}
func (m *mockClient) Crosspost(ctx context.Context, post *reddit.Post, dest string) error {
	return m.crosspost(ctx, post, dest)
}
func (m *mockClient) BotPostsInSub(ctx context.Context, subreddit string) ([]*reddit.Post, error) {
	return m.botPostsInSub(ctx, subreddit)
}
func (m *mockClient) FetchFlair(ctx context.Context, postID string) (string, error) {
	return m.fetchFlair(ctx, postID)
}
func (m *mockClient) DeletePost(ctx context.Context, fullID string) error {
	return m.deletePost(ctx, fullID)
}

// --- extractRedditPostID ---

func TestExtractRedditPostID(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://www.reddit.com/r/gaming/comments/abc123/some_title/", "abc123"},
		{"https://www.reddit.com/r/gaming/comments/abc123/", "abc123"},
		{"https://www.reddit.com/r/gaming/comments/abc123", "abc123"},
		{"https://old.reddit.com/r/gaming/comments/xyz789/title/", "xyz789"},
		{"https://www.reddit.com/r/gaming/", ""},
		{"https://example.com/no/comments/here", "here"},
		{"", ""},
	}

	for _, tc := range cases {
		got := extractRedditPostID(tc.url)
		if got != tc.want {
			t.Errorf("extractRedditPostID(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// --- crosspost logic ---

func TestCrosspost_UsesNewPostsWhenNoFlairFilter(t *testing.T) {
	post := &reddit.Post{ID: "1", Title: "Hello", Permalink: "/r/src/comments/1/hello/"}
	called := false

	p := newTestProcessor(t, config.Route{
		Source: "src", Destination: "dst",
	}, &mockClient{
		newPosts:    func(_ context.Context, _ string, _ int) ([]*reddit.Post, error) { return []*reddit.Post{post}, nil },
		isDuplicate: func(_ context.Context, _ *reddit.Post, _ string) (bool, error) { return false, nil },
		crosspost:   func(_ context.Context, _ *reddit.Post, _ string) error { called = true; return nil },
	})

	if err := p.crosspost(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("crosspost: %v", err)
	}
	if !called {
		t.Error("expected Crosspost to be called")
	}
}

func TestCrosspost_UsesSearchByFlairWhenFlairFilterSet(t *testing.T) {
	post := &reddit.Post{ID: "1", Title: "OC art", Flair: "Art", Permalink: "/r/src/comments/1/oc/"}
	searchCalled := false

	p := newTestProcessor(t, config.Route{
		Source: "src", Destination: "dst",
		Filters: config.Filter{Flair: "Art"},
	}, &mockClient{
		searchByFlair: func(_ context.Context, _, _ string, _ int) ([]*reddit.Post, error) {
			searchCalled = true
			return []*reddit.Post{post}, nil
		},
		isDuplicate: func(_ context.Context, _ *reddit.Post, _ string) (bool, error) { return false, nil },
		crosspost:   func(_ context.Context, _ *reddit.Post, _ string) error { return nil },
	})

	if err := p.crosspost(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("crosspost: %v", err)
	}
	if !searchCalled {
		t.Error("expected SearchByFlair to be called")
	}
}

func TestCrosspost_SkipsDuplicates(t *testing.T) {
	post := &reddit.Post{ID: "1", Title: "Hello"}
	crosspostCalled := false

	p := newTestProcessor(t, config.Route{Source: "src", Destination: "dst"}, &mockClient{
		newPosts:    func(_ context.Context, _ string, _ int) ([]*reddit.Post, error) { return []*reddit.Post{post}, nil },
		isDuplicate: func(_ context.Context, _ *reddit.Post, _ string) (bool, error) { return true, nil },
		crosspost:   func(_ context.Context, _ *reddit.Post, _ string) error { crosspostCalled = true; return nil },
	})

	if err := p.crosspost(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("crosspost: %v", err)
	}
	if crosspostCalled {
		t.Error("Crosspost should not be called for duplicates")
	}
}

func TestCrosspost_TitleRegexFiltersOut(t *testing.T) {
	posts := []*reddit.Post{
		{ID: "1", Title: "[OC] Art piece"},
		{ID: "2", Title: "Random post"},
	}
	crossposted := []string{}

	p := newTestProcessor(t, config.Route{
		Source: "src", Destination: "dst",
		Filters: config.Filter{TitleRegex: `^\[OC\]`},
	}, &mockClient{
		newPosts:    func(_ context.Context, _ string, _ int) ([]*reddit.Post, error) { return posts, nil },
		isDuplicate: func(_ context.Context, _ *reddit.Post, _ string) (bool, error) { return false, nil },
		crosspost: func(_ context.Context, p *reddit.Post, _ string) error {
			crossposted = append(crossposted, p.ID)
			return nil
		},
	})

	if err := p.crosspost(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("crosspost: %v", err)
	}
	if len(crossposted) != 1 || crossposted[0] != "1" {
		t.Errorf("crossposted = %v, want [1]", crossposted)
	}
}

func TestCrosspost_FetchError(t *testing.T) {
	p := newTestProcessor(t, config.Route{Source: "src", Destination: "dst"}, &mockClient{
		newPosts: func(_ context.Context, _ string, _ int) ([]*reddit.Post, error) {
			return nil, errors.New("API down")
		},
	})

	if err := p.crosspost(context.Background(), p.cfg.Routes[0]); err == nil {
		t.Error("expected error when fetch fails")
	}
}

// --- stale flair removal ---

func TestRemoveStaleFlaired_DeletesWhenFlairChanged(t *testing.T) {
	botPost := &reddit.Post{
		FullID: "t3_old1",
		URL:    "https://www.reddit.com/r/src/comments/orig1/title/",
	}
	deleted := []string{}

	p := newTestProcessor(t, config.Route{
		Source: "src", Destination: "dst",
		Filters: config.Filter{Flair: "Highlight"},
	}, &mockClient{
		botPostsInSub: func(_ context.Context, _ string) ([]*reddit.Post, error) { return []*reddit.Post{botPost}, nil },
		fetchFlair:    func(_ context.Context, _ string) (string, error) { return "Discussion", nil },
		deletePost:    func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	})

	if err := p.removeStaleFlaired(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("removeStaleFlaired: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "t3_old1" {
		t.Errorf("deleted = %v, want [t3_old1]", deleted)
	}
}

func TestRemoveStaleFlaired_KeepsWhenFlairMatches(t *testing.T) {
	botPost := &reddit.Post{
		FullID: "t3_keep1",
		URL:    "https://www.reddit.com/r/src/comments/orig1/title/",
	}
	deleted := []string{}

	p := newTestProcessor(t, config.Route{
		Source: "src", Destination: "dst",
		Filters: config.Filter{Flair: "Highlight"},
	}, &mockClient{
		botPostsInSub: func(_ context.Context, _ string) ([]*reddit.Post, error) { return []*reddit.Post{botPost}, nil },
		fetchFlair:    func(_ context.Context, _ string) (string, error) { return "Highlight", nil },
		deletePost:    func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	})

	if err := p.removeStaleFlaired(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("removeStaleFlaired: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("expected no deletions, got %v", deleted)
	}
}

func TestRemoveStaleFlaired_SkipsNonRedditURLs(t *testing.T) {
	botPost := &reddit.Post{
		FullID: "t3_ext1",
		URL:    "https://example.com/some/external/link",
	}
	deleted := []string{}

	p := newTestProcessor(t, config.Route{
		Source: "src", Destination: "dst",
		Filters: config.Filter{Flair: "Highlight"},
	}, &mockClient{
		botPostsInSub: func(_ context.Context, _ string) ([]*reddit.Post, error) { return []*reddit.Post{botPost}, nil },
		fetchFlair:    func(_ context.Context, _ string) (string, error) { return "", nil },
		deletePost:    func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	})

	if err := p.removeStaleFlaired(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("removeStaleFlaired: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("expected no deletions for external URL, got %v", deleted)
	}
}

func TestRemoveStaleFlaired_CaseInsensitiveFlair(t *testing.T) {
	botPost := &reddit.Post{
		FullID: "t3_ci1",
		URL:    "https://www.reddit.com/r/src/comments/orig1/title/",
	}
	deleted := []string{}

	p := newTestProcessor(t, config.Route{
		Source: "src", Destination: "dst",
		Filters: config.Filter{Flair: "Highlight"},
	}, &mockClient{
		botPostsInSub: func(_ context.Context, _ string) ([]*reddit.Post, error) { return []*reddit.Post{botPost}, nil },
		fetchFlair:    func(_ context.Context, _ string) (string, error) { return "HIGHLIGHT", nil },
		deletePost:    func(_ context.Context, id string) error { deleted = append(deleted, id); return nil },
	})

	if err := p.removeStaleFlaired(context.Background(), p.cfg.Routes[0]); err != nil {
		t.Fatalf("removeStaleFlaired: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("expected no deletion for case-insensitive match, got %v", deleted)
	}
}

// newTestProcessor builds a Processor with a single route and the given mock client,
// compiling any filter regexes.
func newTestProcessor(t *testing.T, route config.Route, client redditClient) *Processor {
	t.Helper()
	if err := route.Filters.CompileForTest(); err != nil {
		t.Fatalf("compile filter: %v", err)
	}
	cfg := &config.Config{Routes: []config.Route{route}}
	return &Processor{client: client, cfg: cfg}
}
