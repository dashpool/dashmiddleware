// Package dashmiddleware a plugin integrate Dash apps into Dashpool via a Middleware.
package dashmiddleware

import (
	"context"
	"net/http"
)

// Config the plugin configuration.
type Config struct {
	mongohost string ``
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		mongohost: "",
	}
}

// DashMiddleware a DashMiddleware plugin.
type DashMiddleware struct {
	next      http.Handler
	mongohost string
	name      string
}

// New created a new DashMiddleware plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &DashMiddleware{
		mongohost: config.mongohost,
		next:      next,
		name:      name,
	}, nil
}

func (c *DashMiddleware) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	c.next.ServeHTTP(rw, req)
}
