package config

import (
	"os"
	"testing"
)

func TestFilterMatch(t *testing.T) {
	cases := []struct {
		name       string
		titleRegex string
		flair      string
		title      string
		postFlair  string
		want       bool
	}{
		{name: "empty filter matches anything", want: true},
		{name: "regex match", titleRegex: `^\[Review\]`, title: "[Review] Great game", want: true},
		{name: "regex no match", titleRegex: `^\[Review\]`, title: "Random post", want: false},
		{name: "flair match exact", flair: "Highlight", postFlair: "Highlight", want: true},
		{name: "flair match case-insensitive", flair: "Highlight", postFlair: "highlight", want: true},
		{name: "flair no match", flair: "Highlight", postFlair: "Discussion", want: false},
		{name: "flair filter, empty post flair", flair: "Highlight", postFlair: "", want: false},
		{name: "both match", titleRegex: `^\[Review\]`, flair: "Review", title: "[Review] Good", postFlair: "Review", want: true},
		{name: "title match flair no match", titleRegex: `^\[Review\]`, flair: "Review", title: "[Review] Good", postFlair: "Discussion", want: false},
		{name: "title no match flair match", titleRegex: `^\[Review\]`, flair: "Review", title: "Random", postFlair: "Review", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := &Filter{TitleRegex: tc.titleRegex, Flair: tc.flair}
			if err := f.compile(); err != nil {
				t.Fatalf("compile: %v", err)
			}
			if got := f.Match(tc.title, tc.postFlair); got != tc.want {
				t.Errorf("Match(%q, %q) = %v, want %v", tc.title, tc.postFlair, got, tc.want)
			}
		})
	}
}

func TestFilterCompile_InvalidRegex(t *testing.T) {
	f := &Filter{TitleRegex: `[invalid`}
	if err := f.compile(); err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestCredentialsValidate(t *testing.T) {
	full := Credentials{ClientID: "a", ClientSecret: "b", Username: "c", Password: "d"}
	if err := full.Validate(); err != nil {
		t.Errorf("full credentials: unexpected error: %v", err)
	}

	empty := Credentials{}
	if err := empty.Validate(); err == nil {
		t.Error("empty credentials: expected error, got nil")
	}

	partial := Credentials{ClientID: "a"}
	err := partial.Validate()
	if err == nil {
		t.Fatal("partial credentials: expected error, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"REDDIT_CLIENT_SECRET", "REDDIT_USERNAME", "REDDIT_PASSWORD"} {
		if !contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if contains(msg, "REDDIT_CLIENT_ID") {
		t.Errorf("error %q should not mention REDDIT_CLIENT_ID (it was set)", msg)
	}
}

func TestLoad_Valid(t *testing.T) {
	f := writeTempConfig(t, `
routes:
  - source: gaming
    destination: my_gaming
    filters:
      title_regex: '^\[OC\]'
      flair: Art
`)
	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Routes) != 1 {
		t.Fatalf("want 1 route, got %d", len(cfg.Routes))
	}
	r := cfg.Routes[0]
	if r.Source != "gaming" || r.Destination != "my_gaming" {
		t.Errorf("unexpected route: %+v", r)
	}
	if r.Filters.Flair != "Art" {
		t.Errorf("want flair Art, got %q", r.Filters.Flair)
	}
}

func TestLoad_InvalidRegex(t *testing.T) {
	f := writeTempConfig(t, `
routes:
  - source: gaming
    destination: my_gaming
    filters:
      title_regex: '[invalid'
`)
	if _, err := Load(f); err == nil {
		t.Error("expected error for invalid regex, got nil")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	if _, err := Load("/nonexistent/path/config.yaml"); err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoad_CredentialsFromEnv(t *testing.T) {
	t.Setenv("REDDIT_CLIENT_ID", "testid")
	t.Setenv("REDDIT_CLIENT_SECRET", "testsecret")
	t.Setenv("REDDIT_USERNAME", "testuser")
	t.Setenv("REDDIT_PASSWORD", "testpass")

	f := writeTempConfig(t, "routes: []\n")
	cfg, err := Load(f)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Creds.ClientID != "testid" {
		t.Errorf("ClientID = %q, want testid", cfg.Creds.ClientID)
	}
}

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()
	return f.Name()
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
