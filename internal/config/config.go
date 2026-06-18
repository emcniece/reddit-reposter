package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type Credentials struct {
	ClientID     string
	ClientSecret string
	Username     string
	Password     string
}

type Filter struct {
	TitleRegex string `yaml:"title_regex"`
	Flair      string `yaml:"flair"`

	titleRe *regexp.Regexp
}

type Route struct {
	Source      string `yaml:"source"`
	Destination string `yaml:"destination"`
	Filters     Filter `yaml:"filters"`
}

type Config struct {
	Routes []Route `yaml:"routes"`
	Creds  Credentials
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.Creds = credentialsFromEnv()

	for i := range cfg.Routes {
		if err := cfg.Routes[i].Filters.compile(); err != nil {
			return nil, fmt.Errorf("route %d filter: %w", i, err)
		}
	}

	return &cfg, nil
}

func credentialsFromEnv() Credentials {
	return Credentials{
		ClientID:     os.Getenv("REDDIT_CLIENT_ID"),
		ClientSecret: os.Getenv("REDDIT_CLIENT_SECRET"),
		Username:     os.Getenv("REDDIT_USERNAME"),
		Password:     os.Getenv("REDDIT_PASSWORD"),
	}
}

func (f *Filter) compile() error {
	if f.TitleRegex == "" {
		return nil
	}
	re, err := regexp.Compile(f.TitleRegex)
	if err != nil {
		return fmt.Errorf("invalid title_regex %q: %w", f.TitleRegex, err)
	}
	f.titleRe = re
	return nil
}

// Match returns true if the post passes all configured filters.
// An empty filter matches everything.
func (f *Filter) Match(title, flair string) bool {
	if f.titleRe != nil && !f.titleRe.MatchString(title) {
		return false
	}
	if f.Flair != "" && flair != f.Flair {
		return false
	}
	return true
}

func (c *Credentials) Validate() error {
	missing := []string{}
	if c.ClientID == "" {
		missing = append(missing, "REDDIT_CLIENT_ID")
	}
	if c.ClientSecret == "" {
		missing = append(missing, "REDDIT_CLIENT_SECRET")
	}
	if c.Username == "" {
		missing = append(missing, "REDDIT_USERNAME")
	}
	if c.Password == "" {
		missing = append(missing, "REDDIT_PASSWORD")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missing)
	}
	return nil
}
