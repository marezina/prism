package gh

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/go-github/v92/github"
	"github.com/shurcooL/githubv4"
)

// Options configure a Client.
type Options struct {
	Token     string    // OAuth or classic PAT; never logged
	Version   string    // PRism version for the User-Agent
	Endpoints Endpoints // zero value means DotCom
	Logger    *slog.Logger

	// Transport is the underlying RoundTripper (tests); default http.DefaultTransport.
	Transport http.RoundTripper
}

// Client bundles the REST and GraphQL clients over one HTTP client.
type Client struct {
	REST    *github.Client
	GraphQL *githubv4.Client

	http      *http.Client
	endpoints Endpoints
	limits    *RateLimits
	log       *slog.Logger
}

// New builds a Client. It makes no requests.
func New(o Options) (*Client, error) {
	return newClient(o, time.Now, sleepCtx)
}

func newClient(o Options, now func() time.Time, sleep func(context.Context, time.Duration) error) (*Client, error) {
	if o.Endpoints == (Endpoints{}) {
		o.Endpoints = DotCom
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	if o.Transport == nil {
		o.Transport = http.DefaultTransport
	}
	if o.Version == "" {
		return nil, errors.New("gh: Options.Version is required for the User-Agent")
	}

	limits := newRateLimits()
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &transport{
			base:      o.Transport,
			token:     o.Token,
			userAgent: "PRism/" + o.Version,
			log:       o.Logger,
			limits:    limits,
			now:       now,
			sleep:     sleep,
		},
	}

	rest, err := github.NewClient(
		github.WithHTTPClient(httpClient),
		github.WithURLs(&o.Endpoints.API, &o.Endpoints.API),
	)
	if err != nil {
		return nil, fmt.Errorf("gh: REST client: %w", err)
	}

	return &Client{
		REST:      rest,
		GraphQL:   githubv4.NewEnterpriseClient(o.Endpoints.GraphQL, httpClient),
		http:      httpClient,
		endpoints: o.Endpoints,
		limits:    limits,
		log:       o.Logger,
	}, nil
}

// RateLimits exposes the quotas seen so far.
func (c *Client) RateLimits() *RateLimits { return c.limits }

// Endpoints returns the endpoints this client talks to.
func (c *Client) Endpoints() Endpoints { return c.endpoints }
