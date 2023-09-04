// Package dashmiddleware a plugin integrate Dash apps into Dashpool via a Middleware.
package dashmiddleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// Config the plugin configuration.
type Config struct {
	TrackURL string
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		TrackURL: "http://backand.dashpool-system:8080/track",
	}
}

// DashMiddleware a DashMiddleware plugin.
type DashMiddleware struct {
	next     http.Handler
	trackURL string
	name     string
}

// New creates a new DashMiddleware plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &DashMiddleware{
		trackURL: config.TrackURL,
		next:     next,
		name:     name,
	}, nil
}

func (c *DashMiddleware) ServeHTTP(responseWriter http.ResponseWriter, req *http.Request) {
	// Use the context from the incoming request
	ctx := req.Context()

	_, cancel := context.WithTimeout(ctx, 10)
	defer cancel()

	// Continue the request down the middleware chain
	c.next.ServeHTTP(responseWriter, req)

	// Define the JSON payload to send in the request body
	payload := map[string]interface{}{
		"Request": "SomeRequestValue",
		"Result":  "SomeResultValue",
		"URL":     req.URL.String(),
		"User":    "SomeUserValue",
	}

	// Marshal the payload into a JSON string
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		http.Error(responseWriter, "Failed to create JSON payload", http.StatusInternalServerError)
		return
	}

	// Make a request to the external REST API to track the request
	resp, err := http.Post(c.trackURL, "application/json", bytes.NewBuffer(payloadJSON))
	if err != nil {
		http.Error(responseWriter, "Failed to track request", http.StatusInternalServerError)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return
		}
	}()

	// Check the response status code from the external API
	if resp.StatusCode != http.StatusOK {
		http.Error(responseWriter, "Failed to track request", http.StatusInternalServerError)
		return
	}
}
