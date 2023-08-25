package dashmiddleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dashpool/dashmiddleware"
)

func TestDemo(t *testing.T) {
	cfg := dashmiddleware.CreateConfig()
	cfg.mongohost = "mongo:2701"

	ctx := context.Background()
	next := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {})

	handler, err := dashmiddleware.New(ctx, next, cfg, "dashmiddleware-plugin")
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost", nil)
	if err != nil {
		t.Fatal(err)
	}

	handler.ServeHTTP(recorder, req)

}
