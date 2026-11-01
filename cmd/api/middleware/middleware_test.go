package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/logger"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func makeToken(secret string, valid bool) string {
	claims := jwt.MapClaims{
		"sub":  "user-1",
		"role": "client",
		"exp":  time.Now().Add(time.Hour).Unix(),
	}
	if !valid {
		claims["exp"] = time.Now().Add(-time.Hour).Unix()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(secret))
	return signed
}

// --- Auth middleware ---

func TestAuth_NoHeader(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_InvalidFormat(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "InvalidToken")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_InvalidToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_ExpiredToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken("secret", false))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_ValidToken(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken("secret", true))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuth_WrongSecret(t *testing.T) {
	r := gin.New()
	r.Use(Auth("correct-secret"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken("wrong-secret", true))
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuth_RecusaTokenSemExpOuForaDoHS256(t *testing.T) {
	r := gin.New()
	r.Use(Auth("secret"))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	semExp, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "user-1", "role": "admin"}).SignedString([]byte("secret"))
	hs512, _ := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"sub": "user-1", "role": "admin", "exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte("secret"))

	for name, tok := range map[string]string{"sem exp": semExp, "HS512": hs512} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("%s: expected 401, got %d", name, w.Code)
		}
	}
}

// --- CORS middleware ---

func TestCORS_Allowlist(t *testing.T) {
	r := gin.New()
	r.Use(CORS([]string{"https://painel.oficina.com/"}))
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://painel.oficina.com")
	r.ServeHTTP(w, req)
	if w.Header().Get("Access-Control-Allow-Origin") != "https://painel.oficina.com" || w.Code != http.StatusOK {
		t.Fatalf("origem permitida: %q %d", w.Header().Get("Access-Control-Allow-Origin"), w.Code)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://malicioso.com")
	r.ServeHTTP(w, req)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("origem fora da lista não pode receber CORS, veio %q", got)
	}
}

func TestCORS_OptionsAborts(t *testing.T) {
	r := gin.New()
	r.Use(CORS(nil))
	r.OPTIONS("/", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestNewRelicContext_SemAgente(t *testing.T) {
	r := gin.New()
	r.Use(NewRelicContext())
	r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d", w.Code)
	}
}

func TestCorrelationID_VaiParaOContexto(t *testing.T) {
	r := gin.New()
	r.Use(CorrelationID())
	var got string
	r.GET("/", func(c *gin.Context) { got = logger.CorrelationID(c.Request.Context()) })
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(CorrelationHeader, "abc-123")
	r.ServeHTTP(w, req)
	if got != "abc-123" || w.Header().Get(CorrelationHeader) != "abc-123" {
		t.Fatalf("correlation id = %q", got)
	}
}
