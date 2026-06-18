package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	goreddit "github.com/vartanbeno/go-reddit/v2/reddit"
	"golang.org/x/oauth2"

	"github.com/emcniece/reddit-reposter/internal/config"
)

const (
	userAgent   = "golang:reddit-reposter:v1.0 (by /u/reddit-reposter-bot)"
	oauthBase   = "https://oauth.reddit.com"
	tokenURL    = "https://www.reddit.com/api/v1/access_token"
)

type Post struct {
	ID        string
	FullID    string
	Title     string
	URL       string
	Permalink string
	Flair     string
	IsSelf    bool
	NSFW      bool
}

type Client struct {
	inner    *goreddit.Client
	rawHTTP  *http.Client
	username string
}

func New(creds config.Credentials) (*Client, error) {
	oauthCfg := &oauth2.Config{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL:  tokenURL,
			AuthStyle: oauth2.AuthStyleInHeader,
		},
	}
	token, err := oauthCfg.PasswordCredentialsToken(context.Background(), creds.Username, creds.Password)
	if err != nil {
		return nil, fmt.Errorf("reddit oauth: %w", err)
	}

	rawHTTP := oauthCfg.Client(context.Background(), token)
	rawHTTP.Transport = &uaTransport{inner: rawHTTP.Transport}

	inner, err := goreddit.NewClient(goreddit.Credentials{
		ID:       creds.ClientID,
		Secret:   creds.ClientSecret,
		Username: creds.Username,
		Password: creds.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("create reddit client: %w", err)
	}

	return &Client{inner: inner, rawHTTP: rawHTTP, username: creds.Username}, nil
}

// NewPosts returns the newest posts from a subreddit. Use when no flair filter is set.
func (c *Client) NewPosts(ctx context.Context, subreddit string, limit int) ([]*Post, error) {
	posts, _, err := c.inner.Subreddit.NewPosts(ctx, subreddit, &goreddit.ListOptions{Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("fetch new posts from r/%s: %w", subreddit, err)
	}
	return fromLibPosts(posts), nil
}

// SearchByFlair returns posts in a subreddit that currently carry the given flair,
// sorted by new. Use when a flair filter is configured — Reddit's search handles
// flair matching server-side, so newly-flaired posts appear on subsequent runs.
func (c *Client) SearchByFlair(ctx context.Context, subreddit, flair string, limit int) ([]*Post, error) {
	query := fmt.Sprintf(`flair:"%s"`, flair)
	posts, _, err := c.inner.Subreddit.SearchPosts(ctx, query, subreddit, &goreddit.ListPostSearchOptions{
		ListPostOptions: goreddit.ListPostOptions{},
		Sort:            "new",
	})
	if err != nil {
		return nil, fmt.Errorf("search r/%s flair=%q: %w", subreddit, flair, err)
	}
	out := fromLibPosts(posts)
	// Set flair on all results — the search guarantees they have it, the library just doesn't expose it.
	for _, p := range out {
		p.Flair = flair
	}
	return out, nil
}

// FetchFlair fetches the current link flair of a post by ID using a raw API call,
// since go-reddit's Post struct does not expose link_flair_text.
func (c *Client) FetchFlair(ctx context.Context, postID string) (string, error) {
	url := fmt.Sprintf("%s/by_id/t3_%s", oauthBase, postID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.rawHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch post %s: %w", postID, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch post %s: HTTP %d", postID, resp.StatusCode)
	}

	var body struct {
		Data struct {
			Children []struct {
				Data struct {
					LinkFlairText string `json:"link_flair_text"`
				} `json:"data"`
			} `json:"children"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode post %s: %w", postID, err)
	}
	if len(body.Data.Children) == 0 {
		return "", fmt.Errorf("post %s not found", postID)
	}
	return body.Data.Children[0].Data.LinkFlairText, nil
}

// BotPostsInSub returns posts submitted by the authenticated bot account in the given subreddit.
func (c *Client) BotPostsInSub(ctx context.Context, subreddit string) ([]*Post, error) {
	all, _, err := c.inner.User.Posts(ctx, &goreddit.ListUserOverviewOptions{
		Sort: "new",
		ListOptions: goreddit.ListOptions{Limit: 100},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch bot posts: %w", err)
	}

	var out []*Post
	for _, p := range all {
		if strings.EqualFold(p.SubredditName, subreddit) {
			out = append(out, fromLibPost(p))
		}
	}
	return out, nil
}

// DeletePost removes a post by its full ID (e.g. "t3_abc123").
func (c *Client) DeletePost(ctx context.Context, fullID string) error {
	_, err := c.inner.Post.Delete(ctx, fullID)
	if err != nil {
		return fmt.Errorf("delete post %s: %w", fullID, err)
	}
	return nil
}

// Crosspost submits a link post to destSubreddit pointing at the original post's permalink.
// go-reddit does not expose Reddit's native crosspost endpoint; a link submission to the
// original reddit.com URL achieves the same visual effect.
func (c *Client) Crosspost(ctx context.Context, post *Post, destSubreddit string) error {
	url := "https://www.reddit.com" + post.Permalink
	_, _, err := c.inner.Post.SubmitLink(ctx, goreddit.SubmitLinkRequest{
		Subreddit: destSubreddit,
		Title:     post.Title,
		URL:       url,
		Resubmit:  false,
	})
	if err != nil {
		return fmt.Errorf("crosspost to r/%s: %w", destSubreddit, err)
	}
	return nil
}

// IsDuplicate returns true if the post has already been crossposted to destSubreddit.
func (c *Client) IsDuplicate(ctx context.Context, post *Post, destSubreddit string) (bool, error) {
	_, dupes, _, err := c.inner.Post.Duplicates(ctx, post.ID, &goreddit.ListDuplicatePostOptions{
		ListOptions: goreddit.ListOptions{Limit: 100},
	})
	if err != nil {
		return false, fmt.Errorf("check duplicates for %s: %w", post.ID, err)
	}
	for _, d := range dupes {
		if strings.EqualFold(d.SubredditName, destSubreddit) {
			return true, nil
		}
	}
	return false, nil
}

func fromLibPost(p *goreddit.Post) *Post {
	return &Post{
		ID:        p.ID,
		FullID:    p.FullID,
		Title:     p.Title,
		URL:       p.URL,
		Permalink: p.Permalink,
		IsSelf:    p.IsSelfPost,
		NSFW:      p.NSFW,
	}
}

func fromLibPosts(posts []*goreddit.Post) []*Post {
	out := make([]*Post, len(posts))
	for i, p := range posts {
		out[i] = fromLibPost(p)
	}
	return out
}

// uaTransport injects the required User-Agent header on every request.
type uaTransport struct {
	inner http.RoundTripper
}

func (t *uaTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", userAgent)
	return t.inner.RoundTrip(r)
}
