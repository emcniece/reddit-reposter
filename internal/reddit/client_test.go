package reddit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchFlair(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": {
				"children": [
					{"data": {"link_flair_text": "Highlight"}}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := &Client{rawHTTP: srv.Client()}
	// Override the base URL to point at the test server.
	orig := oauthBase
	t.Cleanup(func() { /* oauthBase is a const; URL is built inline in FetchFlair */ _ = orig })

	flair, err := fetchFlairFromURL(context.Background(), c.rawHTTP, srv.URL+"/by_id/t3_abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flair != "Highlight" {
		t.Errorf("flair = %q, want %q", flair, "Highlight")
	}
}

func TestFetchFlair_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": {"children": [{"data": {"link_flair_text": ""}}]}}`))
	}))
	defer srv.Close()

	flair, err := fetchFlairFromURL(context.Background(), srv.Client(), srv.URL+"/by_id/t3_abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if flair != "" {
		t.Errorf("expected empty flair, got %q", flair)
	}
}

func TestFetchFlair_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data": {"children": []}}`))
	}))
	defer srv.Close()

	_, err := fetchFlairFromURL(context.Background(), srv.Client(), srv.URL+"/by_id/t3_abc123")
	if err == nil {
		t.Error("expected error for empty children, got nil")
	}
}

func TestFetchFlair_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := fetchFlairFromURL(context.Background(), srv.Client(), srv.URL+"/by_id/t3_abc123")
	if err == nil {
		t.Error("expected error for non-200 response, got nil")
	}
}

func TestUATransport(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: &uaTransport{inner: http.DefaultTransport},
	}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	if gotUA != userAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, userAgent)
	}
}
