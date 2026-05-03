package x

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/alnah/moth/internal/config"
	"github.com/alnah/moth/internal/content"
	"github.com/alnah/moth/internal/httpclient"
)

func TestLookupUserByUsernameSendsExpectedRequestAndMapsSocialProfile(t *testing.T) {
	var requests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := atomic.AddInt32(&requests, 1); got > 1 {
			t.Fatalf("LookupUserByUsername() made hidden request %d to %s", got, r.URL.String())
		}
		if strings.Contains(r.URL.Path, "/tweets") || strings.Contains(r.URL.Path, "/2/tweets") {
			t.Fatalf("LookupUserByUsername() fetched posts through %s", r.URL.String())
		}
		assertXBearerRequest(t, r, "/2/users/by/username/alnah")
		query := r.URL.Query()
		assertQueryContains(t, query.Get("user.fields"), "id", "user.fields")
		assertQueryContains(t, query.Get("user.fields"), "username", "user.fields")
		assertQueryContains(t, query.Get("user.fields"), "name", "user.fields")
		if got := query.Get("tweet.fields"); got != "" {
			t.Fatalf("tweet.fields = %q, want empty; username lookup must not request posts", got)
		}
		if got := query.Get("expansions"); got != "" {
			t.Fatalf("expansions = %q, want empty; username lookup must not enrich posts", got)
		}
		writeXJSON(t, w, `{
			"data": {"id":"2244994945", "username":"alnah", "name":"Alexis"}
		}`)
	}))
	defer server.Close()

	pack, err := newXTestClient(server).LookupUserByUsername(
		context.Background(),
		UsernameLookupOptions{Username: "alnah"},
	)
	if err != nil {
		t.Fatalf("LookupUserByUsername() error = %v, want nil", err)
	}

	want := content.Pack{
		Type: content.TypeContentPack,
		Items: []content.Item{{
			Kind:  content.KindSocialProfile,
			URL:   "https://x.com/alnah",
			Title: "@alnah",
			Metadata: map[string]any{
				"source":   "x",
				"user_id":  "2244994945",
				"username": "alnah",
				"name":     "Alexis",
			},
		}},
	}
	if diff := cmp.Diff(want, pack, cmpopts.EquateEmpty()); diff != "" {
		t.Fatalf("LookupUserByUsername() mismatch (-want +got):\n%s", diff)
	}
	assertXContentPackJSONWarningsAreArrays(t, pack)
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("LookupUserByUsername() requests = %d, want 1", got)
	}
}

func TestLookupUserByUsernamePathEscapesUsername(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/users/by/username/alice/bob")
		if got, want := r.URL.EscapedPath(), "/2/users/by/username/alice%2Fbob"; got != want {
			t.Fatalf("escaped path = %q, want %q", got, want)
		}
		writeXJSON(t, w, `{"data":{"id":"1", "username":"alice/bob", "name":"Alice Bob"}}`)
	}))
	defer server.Close()

	_, err := newXTestClient(server).LookupUserByUsername(
		context.Background(),
		UsernameLookupOptions{Username: "alice/bob"},
	)
	if err != nil {
		t.Fatalf("LookupUserByUsername(path escape) error = %v, want nil", err)
	}
}

func TestLookupUserByUsernameMissingBearerTokenReturnsNotConfiguredBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("LookupUserByUsername() made request without bearer token to %s", r.URL.String())
	}))
	defer server.Close()

	_, err := NewClient(Config{BaseURL: server.URL}).LookupUserByUsername(
		context.Background(),
		UsernameLookupOptions{Username: "alnah"},
	)
	assertErrorContains(t, err, "not configured")
}

func TestLookupUserByUsernameStatusErrorsUseXErrorStyle(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		body        string
		wantMessage []string
	}{
		{
			name:        "unauthorized",
			statusCode:  http.StatusUnauthorized,
			body:        `{"detail":"invalid bearer"}`,
			wantMessage: []string{"access denied", "status 401", "invalid bearer"},
		},
		{
			name:        "forbidden",
			statusCode:  http.StatusForbidden,
			body:        `{"detail":"client-not-enrolled"}`,
			wantMessage: []string{"access denied", "status 403", "client-not-enrolled"},
		},
		{
			name:        "rate limited",
			statusCode:  http.StatusTooManyRequests,
			body:        `{"title":"Too Many Requests"}`,
			wantMessage: []string{"failed", "status 429", "Too Many Requests"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertXBearerRequest(t, r, "/2/users/by/username/alnah")
				w.WriteHeader(tt.statusCode)
				writeXJSON(t, w, tt.body)
			}))
			defer server.Close()

			client := NewClient(Config{
				Credentials: config.Credentials{XBearerToken: xTestBearerToken},
				BaseURL:     server.URL,
				HTTPClient:  httpclient.New(httpclient.Options{Attempts: 1}),
			})
			_, err := client.LookupUserByUsername(
				context.Background(),
				UsernameLookupOptions{Username: "alnah"},
			)
			for _, want := range tt.wantMessage {
				assertErrorContains(t, err, want)
			}
		})
	}
}

func TestLookupUserByUsernameMalformedResponseReturnsDecodeError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertXBearerRequest(t, r, "/2/users/by/username/alnah")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":`)
	}))
	defer server.Close()

	_, err := newXTestClient(server).LookupUserByUsername(
		context.Background(),
		UsernameLookupOptions{Username: "alnah"},
	)
	assertErrorContains(t, err, "decode")
}
