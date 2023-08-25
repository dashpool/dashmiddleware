// Package dashmiddleware integrates dash apps into Dashpool
package dashmiddleware

import (
	"context"
	"net/http"
)

// Config the plugin configuration.
type Config struct {
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// DashMiddleware a Dash Middleware plugin.
type DashMiddleware struct {
	next        http.Handler
	name        string
}

// New created a new DashMiddleware plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &DashMiddleware{
		next:        next,
		name:        name,
	}, nil
}

func (c *DashMiddleware) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	c.next.ServeHTTP(rw, req)
}


