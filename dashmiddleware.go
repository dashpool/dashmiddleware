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
	"time"
)

// Config the plugin configuration.
type Config struct {
	TrackURL     string   `yaml:"trackurl"`
	LayoutURL    string   `yaml:"layouturl"`
	ResultURL    string   `yaml:"resulturl"`
	RecordedURLs []string `yaml:"recordedurls"`
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		TrackURL:     "http://backend.dashpool-system:8080/track",
		ResultURL:    "http://backend.dashpool-system:8080/result",
		LayoutURL:    "http://backend.dashpool-system:8080/getlayout",
		RecordedURLs: []string{"/_dash-update-component", "/_dash-layout"},
	}
}

// DashMiddleware a DashMiddleware plugin.
type DashMiddleware struct {
	next         http.Handler
	trackURL     string
	layoutURL    string
	resultURL    string
	name         string
	recordedURLs []string
}

// New creates a new DashMiddleware plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &DashMiddleware{
		trackURL:     config.TrackURL,
		layoutURL:    config.LayoutURL,
		resultURL:    config.ResultURL,
		next:         next,
		name:         name,
		recordedURLs: config.RecordedURLs,
	}, nil
}

// LayoutRequestData needed to get a layout from the backend server.
type LayoutRequestData struct {
	Email  []string `json:"email"`
	Layout string   `json:"layout"`
	Frame  string   `json:"frame"`
}

// Define the regular expressions globally.
var (
	splitRegexp  = regexp.MustCompile(` *([^=;]+?) *=[^;]+`)
	frameRegex   = regexp.MustCompile(`(?:.*[?&]frame=)([^&]+)`)
	layoutRegex  = regexp.MustCompile(`(?:.*[?&]layout=)([^&]+)`)
	baseURLRegex = regexp.MustCompile(`https:\/\/[^\/]+(.+?)\/\?`)
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
	// Start a timer to measure the duration
	var duration float64
	startTime := time.Now()

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

	// Get the long callback header
	longcallback := req.Header.Values("X-Longcallback")
	req.Header.Del("X-Longcallback")
	isLongCallback := len(longcallback) > 0

	// Get the frame info from the referrer
	referer := req.Header.Get("Referer")
	matches := frameRegex.FindStringSubmatch(referer)
	frame := ""
	if len(matches) > 1 {
		frame = matches[1]
	}
	matches = layoutRegex.FindStringSubmatch(referer)
	layout := ""
	if len(matches) > 1 {
		layout = matches[1]
	}
	matches = baseURLRegex.FindStringSubmatch(referer)
	refererBase := ""
	if len(matches) > 1 {
		refererBase = matches[1]
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

	// If the layout is not empty and the URL matches, send the request to layoutURL
	if layout != "" && strings.HasSuffix(url, "/_dash-layout") {
		requestData := LayoutRequestData{
			Email:  email,
			Layout: layout,
			Frame:  frame,
		}

		// Serialize the request data to JSON
		requestBody, jsonReqErr := json.Marshal(requestData)
		if jsonReqErr != nil {
			log.Printf("Failed to serialize request data to JSON: %v", jsonReqErr)
			return
		}

		resp, postErr := http.Post(c.layoutURL, "application/json", bytes.NewBuffer(requestBody))
		if postErr != nil {
			log.Printf("Failed to send request to layoutURL: %v", postErr)
			return
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				log.Printf("Error closing response body: %v", closeErr)
			}
		}()

		// Check the response status code from the external API
		if resp.StatusCode != http.StatusOK {
			log.Printf("Failed to send request to layoutURL. Status Code: %d", resp.StatusCode)
			return
		}

		// Copy the response from resp to responseWriter and return
		layoutBody, readAllErr := io.ReadAll(resp.Body)
		if readAllErr != nil {
			log.Printf("Failed to read layout body: %v", readAllErr)
			return
		}

		responseWriter.Header().Set("Content-Type", "application/json")
		_, err = responseWriter.Write(layoutBody)
		if err != nil {
			log.Printf("Problem sending body to the responsewriter: %v", err)
			return
		}
		return
	}

	// find out if the url is in the recorded ones
	matched := false
	for _, recordedURL := range c.recordedURLs {
		if strings.HasSuffix(url, recordedURL) {
			matched = true
			break
		}
	}

	if !matched {
		c.next.ServeHTTP(responseWriter, req)
		return
	}

	// Create a capturing response writer
	capturingWriter := &CapturingResponseWriter{
		ResponseWriter: responseWriter,
		Body:           []byte{},
	}

	payload := map[string]interface{}{
		"Request":      string(body),
		"URL":          url,
		"longcallback": isLongCallback,
	}

	// Marshal the payload into a JSON string
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to create JSON payload: %v", err)
		return
	}

	// Make a request to the external REST API to check for a recorded result
	cached := false
	resp, err := http.Post(c.resultURL, "application/json", bytes.NewBuffer(payloadJSON))
	if err != nil {
		log.Printf("Failed to get cached request: %v", err)
	}

	if resp.StatusCode == http.StatusOK {
		cached = true
		// copy the header
		for key, values := range resp.Header {
			for _, value := range values {
				responseWriter.Header().Add(key, value)
			}
		}

		// Set the status code
		responseWriter.WriteHeader(http.StatusOK)

		// Capture the response and use it as the response
		_, copyErr := io.Copy(capturingWriter, resp.Body)
		if copyErr != nil {
			log.Printf("Failed to copy response body: %v", copyErr)
			return
		}
		closeErr := resp.Body.Close()
		if closeErr != nil {
			log.Printf("Failed to close response: %v", closeErr)
			return
		}
	} else {
		// If we have a long callback, we send back a 202 and put the request in the queue
		if isLongCallback {
			responseWriter.WriteHeader(http.StatusAccepted)
			return
		}

		// Continue the request down the middleware chain with the capturing response writer
		c.next.ServeHTTP(capturingWriter, req)
	}

	// Calculate the duration
	duration = time.Since(startTime).Seconds()

	// Define the JSON payload to send in the request body
	payload = map[string]interface{}{
		"Request":     string(body),
		"Result":      string(capturingWriter.Body),
		"URL":         url,
		"Email":       email,
		"Groups":      groups,
		"Frame":       frame,
		"Cached":      cached,
		"Duration":    duration,
		"RefererBase": refererBase,
	}

	// Marshal the payload into a JSON string
	payloadJSON, err = json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to create JSON payload: %v", err)
		return
	}

	// Create a new request for the external REST API
	trackReq, err := http.NewRequest(http.MethodPost, c.trackURL, bytes.NewBuffer(payloadJSON))
	if err != nil {
		log.Printf("Failed to create API request: %v", err)
		return
	}

	// Copy headers from the original request to the new request
	expires := capturingWriter.ResponseWriter.Header().Get("Expires")
	trackReq.Header.Add("Expires", expires)

	// Set the Content-Type header for the new request
	contentType := capturingWriter.ResponseWriter.Header().Get("Content-Type")
	trackReq.Header.Set("Content-Type", contentType)

	// Check if the data is compressed
	if capturingWriter.ResponseWriter.Header().Get("Content-Encoding") == "gzip" {
		trackReq.Header.Set("Content-Encoding", "gzip")
	}

	// Make a request to the external REST API with headers from the original request
	resp, err = http.DefaultClient.Do(trackReq)
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
