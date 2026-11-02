package httputil

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type bindingItem struct {
	Quantity int `json:"quantity" binding:"gt=0"`
}

type bindingRequest struct {
	Name   string        `json:"name" binding:"required,max=5"`
	Email  string        `json:"email" binding:"omitempty,email"`
	Year   int           `json:"year" binding:"omitempty,min=1900,lte=2100"`
	Status string        `json:"status" binding:"omitempty,oneof=open closed"`
	Tags   []string      `json:"tags" binding:"omitempty,min=2"`
	Items  []bindingItem `json:"items" binding:"omitempty,dive"`
}

type bindingQuery struct {
	Document string `form:"document" binding:"required"`
}

func bindBody(t *testing.T, body string) string {
	t.Helper()
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var req bindingRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			HandleBadRequest(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("%s: expected 400, got %d", body, w.Code)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error.Code != CodeValidation {
		t.Errorf("expected %s, got %s", CodeValidation, resp.Error.Code)
	}
	return resp.Error.Message
}

func TestBindingMessage_CitaCampoDoJSONERegra(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{`{}`, "name: is required"},
		{`{"name":"abcdefg"}`, "name: must have at most 5 characters"},
		{`{"name":"a","email":"x"}`, "email: must be a valid e-mail address"},
		{`{"name":"a","year":1800}`, "year: must be at least 1900"},
		{`{"name":"a","year":2200}`, "year: must be at most 2100"},
		{`{"name":"a","status":"x"}`, "status: must be one of: open, closed"},
		{`{"name":"a","tags":["x"]}`, "tags: must have at least 2 items"},
		{`{"name":"a","items":[{"quantity":0}]}`, "items[0].quantity: must be greater than 0"},
		{`{"name":"a","year":"2020"}`, "year: invalid type, expected number"},
		{`{"name":`, "malformed JSON body"},
		{`{bad`, "malformed JSON body"},
		{``, "request body is empty"},
	}
	for _, tc := range cases {
		got := bindBody(t, tc.body)
		if got != tc.want {
			t.Errorf("%s: expected %q, got %q", tc.body, tc.want, got)
		}
		if strings.Contains(got, "bindingRequest") || strings.Contains(got, "Error:Field") {
			t.Errorf("%s: message exposes validator internals: %q", tc.body, got)
		}
	}
}

func TestBindingMessage_VariosCamposEQuery(t *testing.T) {
	if got := bindBody(t, `{"email":"x"}`); got != "name: is required; email: must be a valid e-mail address" {
		t.Errorf("unexpected message: %q", got)
	}

	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		var q bindingQuery
		if err := c.ShouldBindQuery(&q); err != nil {
			HandleBadRequest(c, err)
		}
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if !strings.Contains(w.Body.String(), `"document: is required"`) {
		t.Errorf("query: unexpected body %s", w.Body.String())
	}
}

func TestBindingMessage_OutrosErrosPassamDireto(t *testing.T) {
	if got := BindingMessage(errors.New("invalid CPF/CNPJ")); got != "invalid CPF/CNPJ" {
		t.Errorf("unexpected message: %q", got)
	}
}
