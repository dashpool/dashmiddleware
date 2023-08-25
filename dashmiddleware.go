// Package dashmiddleware a plugin integrate Dash apps into Dashpool via a Middleware.
package dashmiddleware

import (
	"context"
	"net/http"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Config the plugin configuration.
type Config struct {
	Mongohost  string
	Dashpooldb string
}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{
		Mongohost:  "",
		Dashpooldb: "",
	}
}

// DashMiddleware a DashMiddleware plugin.
type DashMiddleware struct {
	next       http.Handler
	mongohost  string
	dashpooldb string
	name       string
}

// New creates a new DashMiddleware plugin.
func New(_ context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &DashMiddleware{
		mongohost:  config.Mongohost,
		dashpooldb: config.Dashpooldb,
		next:       next,
		name:       name,
	}, nil
}

func (c *DashMiddleware) ServeHTTP(responseWriter http.ResponseWriter, req *http.Request) {
	// Use the context from the incoming request
	ctx := req.Context()

	ctx, cancel := context.WithTimeout(ctx, 10)
	defer cancel()

	// Create a MongoDB client
	clientOptions := options.Client().ApplyURI(c.mongohost)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		http.Error(responseWriter, "Mongo Connection not possible", http.StatusInternalServerError)
		return
	}
	defer func() {
		if disconnectErr := client.Disconnect(ctx); disconnectErr != nil {
			http.Error(responseWriter, "Mongo Disconnected", http.StatusInternalServerError)
		}
	}()

	// Select the MongoDB database using the provided dashpooldb
	db := client.Database(c.dashpooldb)

	// Insert the request data into a MongoDB collection
	// Here, we'll create a 'requests' collection and insert the request URL
	collection := db.Collection("requests")
	_, err = collection.InsertOne(ctx, bson.M{"url": req.URL.String()})
	if err != nil {
		// Handle the error (e.g., log it)
		http.Error(responseWriter, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Continue the request down the middleware chain
	c.next.ServeHTTP(responseWriter, req)
}
