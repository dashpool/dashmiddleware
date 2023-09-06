// Package dashmiddleware a plugin integrate Dash apps into Dashpool via a Middleware.
package dashmiddleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
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
	next        http.Handler
	trackURL    string
	name        string
	splitRegexp *regexp.Regexp
}

// New creates a new DashMiddleware plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &DashMiddleware{
		trackURL:    config.TrackURL,
		next:        next,
		name:        name,
		splitRegexp: regexp.MustCompile(` *([^=;]+?) *=[^;]+`),
	}, nil
}

// CapturingResponseWriter a ResponseWriter that knows its response.
type CapturingResponseWriter struct {
	http.ResponseWriter
	Body []byte
}

func (w *CapturingResponseWriter) Write(b []byte) (int, error) {
	// Capture the response body
	w.Body = append(w.Body, b...)
	return w.ResponseWriter.Write(b)
}

func (c *DashMiddleware) ServeHTTP(responseWriter http.ResponseWriter, req *http.Request) {
	// handle auth cookies
	cookies := req.Header.Values("cookie")
	req.Header.Del("cookie")

	// restore non auth cookies
	for _, cookieLine := range cookies {
		cookies := c.splitRegexp.FindAllStringSubmatch(cookieLine, -1)
		var keep []string
		for _, cookie := range cookies {
			if !strings.HasPrefix(cookie[1], "_oauth2_proxy") {
				keep = append(keep, cookieLine)
			}
		}
		if len(keep) > 0 {
			req.Header.Add("cookie", strings.TrimSpace(strings.Join(keep, ";")))
		}
	}

	// Get user information and remove groups (since they might be long)
	email := req.Header.Values("X-Auth-Request-Email")
	groups := req.Header.Values("X-Auth-Request-Groups")
	req.Header.Del("X-Auth-Request-Groups")

	// Use the context from the incoming request
	ctx := req.Context()

	_, cancel := context.WithTimeout(ctx, 10)
	defer cancel()

	// Read the request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		log.Printf("Failed to read request body: %v", err)
		return
	}
	// Restore the original request body for downstream handlers
	req.Body = io.NopCloser(bytes.NewBuffer(body))

	// Create a capturing response writer
	capturingWriter := &CapturingResponseWriter{
		ResponseWriter: responseWriter, Body: []byte{},
	}

	// Continue the request down the middleware chain
	c.next.ServeHTTP(capturingWriter, req)

	url := req.URL.String()

	// Check if the URL ends with "/_dash-update-component"
	if strings.HasSuffix(url, "/_dash-update-component") {
		// Remove "/_dash-update-component" from the URL
		url = strings.TrimSuffix(url, "/_dash-update-component")

		// Define the JSON payload to send in the request body
		payload := map[string]interface{}{
			"Request": string(body),
			"Result":  string(capturingWriter.Body),
			"URL":     url,
			"Email":   email,
			"Groups":  groups,
		}

		// Marshal the payload into a JSON string
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			log.Printf("Failed to create JSON payload: %v", err)
			return
		}

		// Make a request to the external REST API to track the request
		resp, err := http.Post(c.trackURL, "application/json", bytes.NewBuffer(payloadJSON))
		if err != nil {
			log.Printf("Failed to track request: %v", err)
			return
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body: %v", closeErr)
			}
		}()

		// Check the response status code from the external API
		if resp.StatusCode != http.StatusOK {
			log.Printf("Failed to track request. Status Code: %d", resp.StatusCode)
			return
		}
	}
}
