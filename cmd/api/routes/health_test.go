package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type pinger struct{ err error }

func (p pinger) PingContext(context.Context) error { return p.err }

func TestHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		db         Pinger
		path       string
		wantStatus int
	}{
		{pinger{}, "/health/live", 200},
		{pinger{}, "/health", 200},
		{pinger{}, "/health/ready", 200},
		{pinger{errors.New("down")}, "/health/ready", 503},
		{pinger{errors.New("down")}, "/health/live", 200},
		{nil, "/health/ready", 503},
	} {
		r := gin.New()
		Health(r, tc.db)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.wantStatus {
			t.Errorf("%s: %d", tc.path, w.Code)
		}
	}
}
