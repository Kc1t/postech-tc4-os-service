package httputil

import (
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// O validator do gin passa a nomear o campo como no JSON (ou na query/URI), e não pelo nome
// do campo da struct Go. Precisa rodar antes da primeira validação de cada tipo, porque o
// validator guarda o nome em cache por tipo.
func init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		v.RegisterTagNameFunc(fieldTagName)
	}
}

func fieldTagName(f reflect.StructField) string {
	for _, tag := range []string{"json", "form", "uri"} {
		name, _, _ := strings.Cut(f.Tag.Get(tag), ",")
		if name != "" && name != "-" {
			return name
		}
	}
	return ""
}

// BindingMessage traduz o erro de ShouldBind* para uma mensagem que cita o campo e a regra,
// sem expor nomes de structs nem o texto interno do validator.
func BindingMessage(err error) string {
	var verrs validator.ValidationErrors
	var typeErr *json.UnmarshalTypeError
	var syntaxErr *json.SyntaxError
	switch {
	case errors.As(err, &verrs):
		parts := make([]string, 0, len(verrs))
		for _, fe := range verrs {
			parts = append(parts, fieldPath(fe)+": "+ruleMessage(fe))
		}
		return strings.Join(parts, "; ")
	case errors.As(err, &typeErr):
		return typeErr.Field + ": invalid type, expected " + jsonType(typeErr.Type)
	case errors.As(err, &syntaxErr), errors.Is(err, io.ErrUnexpectedEOF):
		return "malformed JSON body"
	case errors.Is(err, io.EOF):
		return "request body is empty"
	}
	return err.Error()
}

// fieldPath tira o nome da struct raiz: "CreateRequest.items[0].quantity" vira "items[0].quantity".
func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if _, rest, ok := strings.Cut(ns, "."); ok {
		return rest
	}
	return ns
}

func ruleMessage(fe validator.FieldError) string {
	p := fe.Param()
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid e-mail address"
	case "uuid", "uuid4":
		return "must be a UUID"
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(p, " ", ", ")
	case "gt":
		return "must be greater than " + p
	case "gte":
		return "must be at least " + p
	case "lt":
		return "must be less than " + p
	case "lte":
		return "must be at most " + p
	case "min":
		return sizeMessage(fe.Kind(), "at least", p)
	case "max":
		return sizeMessage(fe.Kind(), "at most", p)
	case "len":
		return sizeMessage(fe.Kind(), "exactly", p)
	}
	return "is invalid (rule " + fe.Tag() + ")"
}

func sizeMessage(kind reflect.Kind, bound, p string) string {
	switch kind {
	case reflect.String:
		return "must have " + bound + " " + p + " characters"
	case reflect.Slice, reflect.Array, reflect.Map:
		return "must have " + bound + " " + p + " items"
	default:
		return "must be " + bound + " " + p
	}
}

func jsonType(t reflect.Type) string {
	if t == nil {
		return "another type"
	}
	switch t.Kind() {
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	case reflect.Slice, reflect.Array:
		return "array"
	default:
		return "object"
	}
}
