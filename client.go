package mvpp_openapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
)

// DefaultAPIURL is the API endpoint used by this preview release.
const DefaultAPIURL = "http://managed-agents-gray.modelverse.cn"

// New selects the explicit base URL, then MVPP_API_URL, then DefaultAPIURL.
// When apiKey is empty, it reads MVPP_API_KEY. Redirects are disabled so that
// the API key stays on the selected origin.
func New(apiKey, baseURL string) (*ClientWithResponses, error) {
	if apiKey == "" {
		apiKey = os.Getenv("MVPP_API_KEY")
	}
	if apiKey == "" {
		return nil, errors.New("apiKey or MVPP_API_KEY is required")
	}
	if baseURL == "" {
		var configured bool
		baseURL, configured = os.LookupEnv("MVPP_API_URL")
		if !configured {
			baseURL = DefaultAPIURL
		}
	}

	endpoint, err := url.Parse(baseURL)
	if err != nil || endpoint.Hostname() == "" || endpoint.User != nil ||
		endpoint.RawQuery != "" || endpoint.ForceQuery || endpoint.Fragment != "" ||
		(endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return nil, errors.New("baseURL must be an HTTP(S) URL without credentials, query, or fragment")
	}

	return NewClientWithResponses(baseURL, WithRequestEditorFn(func(_ context.Context, request *http.Request) error {
		request.Header.Set("x-api-key", apiKey)
		return nil
	}), WithHTTPClient(&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}))
}
