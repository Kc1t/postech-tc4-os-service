package httputil

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestUUIDParams(t *testing.T) {
	r := gin.New()
	g := r.Group("", UUIDParams("id"))
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	g.GET("/x/:id", ok)
	g.GET("/y", ok)

	cases := []struct {
		path   string
		status int
	}{
		{"/x/11111111-1111-4111-8111-111111111111", http.StatusNoContent},
		{"/x/nao-e-uuid", http.StatusBadRequest},
		{"/x/123", http.StatusBadRequest},
		{"/x/11111111-1111-4111-8111-11111111111", http.StatusBadRequest},
		{"/y", http.StatusNoContent},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: expected %d, got %d", tc.path, tc.status, w.Code)
		}
		if tc.status != http.StatusBadRequest {
			continue
		}
		var resp ErrorResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("%s: corpo inválido: %v", tc.path, err)
		}
		if resp.Error.Code != CodeValidation || !strings.Contains(resp.Error.Message, "'id'") {
			t.Fatalf("%s: resposta inesperada %+v", tc.path, resp.Error)
		}
	}
}
