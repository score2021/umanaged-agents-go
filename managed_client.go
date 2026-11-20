package umanagedagents

import (
	"context"
	"errors"
	"iter"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client holds the generated operations and the authentication context for Managed Agents.
type Client struct {
	API *APIClient
	key string
}

// NewClient reads MANAGED_AGENTS_API_URL and MANAGED_AGENTS_API_KEY (then the legacy MVPP_* names) when its arguments
// are empty. There is no built-in API address (#481).
func NewClient(baseURL, apiKey string) (*Client, error) {
	if baseURL == "" {
		baseURL = env("API_URL")
	}
	if baseURL == "" {
		return nil, errors.New("base URL is required (or MANAGED_AGENTS_API_URL)")
	}
	if apiKey == "" {
		apiKey = env("API_KEY")
	}
	if apiKey == "" {
		return nil, errors.New("api key is required (or MANAGED_AGENTS_API_KEY)")
	}
	target, err := url.Parse(baseURL)
	if err != nil || (target.Scheme != "https" && target.Scheme != "http") ||
		target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return nil, errors.New("base URL must be an HTTP(S) API root")
	}
	cfg := NewConfiguration()
	cfg.Servers = ServerConfigurations{{URL: strings.TrimRight(baseURL, "/")}}
	cfg.HTTPClient = &http.Client{
		Timeout:   30 * time.Second,
		Transport: retryTransport{base: http.DefaultTransport},
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return &Client{API: NewAPIClient(cfg), key: apiKey}, nil
}

// env returns MANAGED_AGENTS_<name>, falling back to the legacy MVPP_<name>; an empty value counts as unset.
func env(name string) string {
	if value := os.Getenv("MANAGED_AGENTS_" + name); value != "" {
		return value
	}
	return os.Getenv("MVPP_" + name)
}

// Context authenticates generated requests without modifying the caller's context.
func (c *Client) Context(ctx context.Context) context.Context {
	return context.WithValue(ctx, ContextAPIKeys, map[string]APIKey{"ApiKey": {Key: c.key}})
}

// WaitTurn polls the accepted input until it is no longer running or the caller's context ends.
// A canceled context ends the wait and leaves the server-side turn active.
func (c *Client) WaitTurn(ctx context.Context, sessionID, inputID string) (*ResponseTurn, error) {
	ctx = c.Context(ctx)
	for {
		turn, _, err := c.API.SessionsAPI.GetTurn(ctx, sessionID, inputID).WaitMs(5000).Execute()
		if err != nil {
			return nil, err
		}
		if turn.Status != "running" {
			return turn, nil
		}
	}
}

// Events iterates the persisted event history and detects a stalled cursor.
func (c *Client) Events(ctx context.Context, sessionID string, limit int64) iter.Seq2[SessionEvent, error] {
	return func(yield func(SessionEvent, error) bool) {
		var cursor string
		seen := make(map[string]bool)
		ctx := c.Context(ctx)
		for {
			request := c.API.SessionsAPI.ListEvents(ctx, sessionID).Limit(limit)
			if cursor != "" {
				request = request.Page(cursor)
			}
			page, _, err := request.Execute()
			if err != nil {
				yield(SessionEvent{}, err)
				return
			}
			for _, event := range page.Data {
				if !yield(event, nil) {
					return
				}
			}
			next, present := page.GetNextPageOk()
			if !present {
				yield(SessionEvent{}, errors.New("event history omitted next_page"))
				return
			}
			if next == nil {
				return
			}
			cursor = *next
			if cursor == "" || seen[cursor] {
				yield(SessionEvent{}, errors.New("event history returned an empty or repeated next_page"))
				return
			}
			seen[cursor] = true
		}
	}
}

type retryTransport struct{ base http.RoundTripper }

func (t retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	replayable := (req.Method == http.MethodGet || req.Header.Get("Idempotency-Key") != "") &&
		(req.Body == nil || req.Body == http.NoBody || req.GetBody != nil)
	attempt := req
	for retry := 0; ; retry++ {
		resp, err := t.base.RoundTrip(attempt)
		if !replayable || retry == 2 || req.Context().Err() != nil || !retryableResponse(resp, err) {
			return resp, err
		}
		delay := time.Duration(1<<retry) * 500 * time.Millisecond
		if resp != nil {
			delay = retryDelay(resp.Header.Get("Retry-After"), delay)
			resp.Body.Close()
		}
		timer := time.NewTimer(delay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
		attempt = req.Clone(req.Context())
		if req.GetBody != nil {
			attempt.Body, err = req.GetBody()
			if err != nil {
				return nil, err
			}
		}
	}
}

func retryableResponse(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	switch resp.StatusCode {
	case 429, 500, 502, 503, 504:
		return true
	default:
		return false
	}
}

func retryDelay(header string, delay time.Duration) time.Duration {
	seconds, err := strconv.Atoi(header)
	if err == nil && seconds >= 0 && seconds <= 86400 {
		return max(delay, time.Duration(seconds)*time.Second)
	}
	if date, err := http.ParseTime(header); err == nil {
		return max(delay, time.Until(date))
	}
	return delay
}
