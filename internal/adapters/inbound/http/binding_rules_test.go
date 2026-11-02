package httputil

import (
	"reflect"
	"testing"

	"github.com/go-playground/validator/v10"
)

// fakeFieldError implementa só o que ruleMessage e fieldPath leem do validator.FieldError.
type fakeFieldError struct {
	validator.FieldError
	ns, tag, param string
	kind           reflect.Kind
}

func (f fakeFieldError) Namespace() string  { return f.ns }
func (f fakeFieldError) Tag() string        { return f.tag }
func (f fakeFieldError) Param() string      { return f.param }
func (f fakeFieldError) Kind() reflect.Kind { return f.kind }

func TestRuleMessage_TodasAsRegras(t *testing.T) {
	cases := []struct {
		tag, param string
		kind       reflect.Kind
		want       string
	}{
		{"uuid", "", reflect.String, "must be a UUID"},
		{"gte", "1", reflect.Int, "must be at least 1"},
		{"lt", "10", reflect.Int, "must be less than 10"},
		{"len", "8", reflect.String, "must have exactly 8 characters"},
		{"len", "3", reflect.Int, "must be exactly 3"},
		{"max", "2", reflect.Map, "must have at most 2 items"},
		{"url", "", reflect.String, "is invalid (rule url)"},
	}
	for _, tc := range cases {
		got := ruleMessage(fakeFieldError{tag: tc.tag, param: tc.param, kind: tc.kind})
		if got != tc.want {
			t.Errorf("%s=%s: expected %q, got %q", tc.tag, tc.param, tc.want, got)
		}
	}
}

func TestFieldPath_SemStructRaiz(t *testing.T) {
	if got := fieldPath(fakeFieldError{ns: "document"}); got != "document" {
		t.Errorf("expected document, got %q", got)
	}
	if got := fieldPath(fakeFieldError{ns: "Request.items[0].code"}); got != "items[0].code" {
		t.Errorf("expected items[0].code, got %q", got)
	}
}

func TestJSONType(t *testing.T) {
	cases := []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(""), "string"},
		{reflect.TypeOf(true), "boolean"},
		{reflect.TypeOf(1.5), "number"},
		{reflect.TypeOf([]int{}), "array"},
		{reflect.TypeOf(struct{}{}), "object"},
		{nil, "another type"},
	}
	for _, tc := range cases {
		if got := jsonType(tc.typ); got != tc.want {
			t.Errorf("%v: expected %q, got %q", tc.typ, tc.want, got)
		}
	}
}

func TestFieldTagName(t *testing.T) {
	type sample struct {
		JSON    string `json:"name,omitempty"`
		Form    string `form:"document"`
		URI     string `uri:"id"`
		Ignored string `json:"-"`
		Plain   string
	}
	typ := reflect.TypeOf(sample{})
	want := []string{"name", "document", "id", "", ""}
	for i, w := range want {
		if got := fieldTagName(typ.Field(i)); got != w {
			t.Errorf("%s: expected %q, got %q", typ.Field(i).Name, w, got)
		}
	}
}
