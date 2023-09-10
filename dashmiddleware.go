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
	TrackURL     string   `yaml:"trackurl"`
	StateURL     string   `yaml:"stateurl"`
	RecordedURLs []string `yaml:"recordedurls"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		TrackURL:     "http://backend.dashpool-system:8080/track",
		StateURL:     "http://backend.dashpool-system:8080/state",
		RecordedURLs: []string{"/_dash-update-component"},
	}
}

// DashMiddleware a DashMiddleware plugin.
type DashMiddleware struct {
	next         http.Handler
	trackURL     string
	stateURL     string
	name         string
	recordedURLs []string
}

// New creates a new DashMiddleware plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &DashMiddleware{
		trackURL:     config.TrackURL,
		stateURL:     config.StateURL,
		next:         next,
		name:         name,
		recordedURLs: config.RecordedURLs,
	}, nil
}

// Define the regular expressions globally.
var (
	splitRegexp = regexp.MustCompile(` *([^=;]+?) *=[^;]+`)
	frameRegex  = regexp.MustCompile(`(?:.*[?&]frame=)([^&]+)`)
	stateRegex  = regexp.MustCompile(`(?:.*[?&]state=)([^&]+)`)
)

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
		cookies := splitRegexp.FindAllStringSubmatch(cookieLine, -1)
		var keep []string
		for _, cookie := range cookies {
			if !strings.HasPrefix(cookie[1], "_oauth2_proxy") {
				keep = append(keep, cookie[0])
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

	// Get the frame info from the referrer
	referer := req.Header.Get("Referer")
	matches := frameRegex.FindStringSubmatch(referer)
	frame := ""
	if len(matches) > 1 {
		frame = matches[1]
	}
	matches = stateRegex.FindStringSubmatch(referer)
	state := ""
	if len(matches) > 1 {
		state = matches[1]
	}

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

	// Check if the URL matches any of the RecordedURLs
	url := req.URL.String()
	matched := false
	for _, recordedURL := range c.recordedURLs {
		if strings.HasSuffix(url, recordedURL) {
			matched = true
			break
		}
	}

	// If the state is not empty and the URL matches, send the request to stateURL
	if state != "" && matched {
		stateURL := c.stateURL + "?state=" + state
		resp, err := http.Post(stateURL, "application/json", bytes.NewBuffer(body)) //nolint
		if err != nil {
			log.Printf("Failed to send request to stateURL: %v", err)
			return
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body: %v", closeErr)
			}
		}()

		// Check the response status code from the external API
		if resp.StatusCode != http.StatusOK {
			log.Printf("Failed to send request to stateURL. Status Code: %d", resp.StatusCode)
			return
		}

		// Copy the response from resp to responseWriter and return
		_, copyErr := io.Copy(responseWriter, resp.Body)
		if copyErr != nil {
			log.Printf("Failed to copy response to responseWriter: %v", copyErr)
		}
		return
	}

	// Create a capturing response writer
	capturingWriter := &CapturingResponseWriter{
		ResponseWriter: responseWriter, Body: []byte{},
	}

	// Continue the request down the middleware chain
	c.next.ServeHTTP(capturingWriter, req)

	// if we have a url request we have to record
	if matched {
		// Define the JSON payload to send in the request body
		payload := map[string]interface{}{
			"Request": string(body),
			"Result":  string(capturingWriter.Body),
			"URL":     url,
			"Email":   email,
			"Groups":  groups,
			"Frame":   frame,
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
