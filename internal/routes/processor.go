package routes

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/emcniece/reddit-reposter/internal/config"
	"github.com/emcniece/reddit-reposter/internal/reddit"
)

const defaultFetchLimit = 25

type redditClient interface {
	NewPosts(ctx context.Context, subreddit string, limit int) ([]*reddit.Post, error)
	SearchByFlair(ctx context.Context, subreddit, flair string, limit int) ([]*reddit.Post, error)
	IsDuplicate(ctx context.Context, post *reddit.Post, destSubreddit string) (bool, error)
	Crosspost(ctx context.Context, post *reddit.Post, destSubreddit string) error
	BotPostsInSub(ctx context.Context, subreddit string) ([]*reddit.Post, error)
	FetchFlair(ctx context.Context, postID string) (string, error)
	DeletePost(ctx context.Context, fullID string) error
}

type Processor struct {
	client redditClient
	cfg    *config.Config
}

func NewProcessor(client *reddit.Client, cfg *config.Config) *Processor {
	return &Processor{client: client, cfg: cfg}
}

// RunAll processes all configured routes once: crosspost new matches, then remove
// crossposts whose source flair has since changed.
func (p *Processor) RunAll(ctx context.Context) error {
	var errs []error
	for _, route := range p.cfg.Routes {
		if err := p.crosspost(ctx, route); err != nil {
			errs = append(errs, fmt.Errorf("crosspost r/%s→r/%s: %w", route.Source, route.Destination, err))
		}
		if route.Filters.Flair != "" {
			if err := p.removeStaleFlaired(ctx, route); err != nil {
				errs = append(errs, fmt.Errorf("cleanup r/%s→r/%s: %w", route.Source, route.Destination, err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%v", errs)
	}
	return nil
}

// crosspost fetches candidate posts from the source subreddit and crossposts any
// that pass filters and haven't already been posted to the destination.
func (p *Processor) crosspost(ctx context.Context, route config.Route) error {
	posts, err := p.fetchCandidates(ctx, route)
	if err != nil {
		return err
	}

	for _, post := range posts {
		if !route.Filters.Match(post.Title, post.Flair) {
			continue
		}

		isDupe, err := p.client.IsDuplicate(ctx, post, route.Destination)
		if err != nil {
			log.Printf("warn: dedup check failed for %s: %v", post.ID, err)
			continue
		}
		if isDupe {
			continue
		}

		if err := p.client.Crosspost(ctx, post, route.Destination); err != nil {
			log.Printf("error: crosspost %s to r/%s: %v", post.ID, route.Destination, err)
			continue
		}
		log.Printf("crossposted %s (%q) → r/%s", post.ID, post.Title, route.Destination)
	}
	return nil
}

// fetchCandidates returns posts to evaluate for crossposting.
// When a flair filter is set, it uses Reddit's search API so that posts whose flair
// was applied after creation are caught on subsequent runs. Falls back to /new otherwise.
func (p *Processor) fetchCandidates(ctx context.Context, route config.Route) ([]*reddit.Post, error) {
	if route.Filters.Flair != "" {
		return p.client.SearchByFlair(ctx, route.Source, route.Filters.Flair, defaultFetchLimit)
	}
	return p.client.NewPosts(ctx, route.Source, defaultFetchLimit)
}

// removeStaleFlaired scans the bot's recent posts in the destination subreddit and
// deletes any whose original source post no longer carries the expected flair.
// This handles flair being removed or changed after a crosspost was made.
func (p *Processor) removeStaleFlaired(ctx context.Context, route config.Route) error {
	botPosts, err := p.client.BotPostsInSub(ctx, route.Destination)
	if err != nil {
		return err
	}

	for _, botPost := range botPosts {
		origID := extractRedditPostID(botPost.URL)
		if origID == "" {
			continue // not a Reddit link we posted
		}

		currentFlair, err := p.client.FetchFlair(ctx, origID)
		if err != nil {
			log.Printf("warn: could not fetch flair for %s: %v", origID, err)
			continue
		}

		if !strings.EqualFold(currentFlair, route.Filters.Flair) {
			if err := p.client.DeletePost(ctx, botPost.FullID); err != nil {
				log.Printf("error: delete stale post %s: %v", botPost.FullID, err)
				continue
			}
			log.Printf("deleted stale crosspost %s (flair was %q, want %q)", botPost.FullID, currentFlair, route.Filters.Flair)
		}
	}
	return nil
}

// extractRedditPostID parses a Reddit post URL and returns the post ID.
// Handles both https://www.reddit.com/r/{sub}/comments/{id}/... and short forms.
func extractRedditPostID(url string) string {
	// Match /comments/{id}/ in the URL path
	const marker = "/comments/"
	_, after, found := strings.Cut(url, marker)
	if !found {
		return ""
	}
	id, _, _ := strings.Cut(after, "/")
	return id
}
