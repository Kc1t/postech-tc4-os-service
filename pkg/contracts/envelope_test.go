package contracts

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNewEnvelopeAndDecode(t *testing.T) {
	meta := Meta{SagaID: "saga-1", OSID: "os-1", CorrelationID: "corr-1"}
	env, err := NewEnvelope(CmdCreateBudget, "os-service", meta, CreateBudget{OSCode: 42, Total: 150})
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if env.MessageID == "" || env.Version != Version || env.Producer != "os-service" || env.OccurredAt.IsZero() {
		t.Fatalf("envelope incompleto: %+v", env)
	}
	if env.SagaID != "saga-1" || env.OSID != "os-1" || env.CorrelationID != "corr-1" {
		t.Fatalf("meta não copiada: %+v", env)
	}

	body, err := env.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(body), `"payload":{"os_code":42`) {
		t.Fatalf("payload deveria ser JSON aninhado, veio %s", body)
	}

	parsed, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := Decode[CreateBudget](parsed)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.OSCode != 42 || got.Total != 150 {
		t.Fatalf("payload decodificado errado: %+v", got)
	}
}

func TestNewEnvelope_PayloadInvalido(t *testing.T) {
	if _, err := NewEnvelope("x", "p", Meta{}, make(chan int)); err == nil {
		t.Fatal("esperava erro ao serializar canal")
	}
}

func TestReply(t *testing.T) {
	in := Envelope{MessageID: "m-1", SagaID: "s", OSID: "o", CorrelationID: "c"}
	r := in.Reply()
	if r.CausationID != "m-1" || r.SagaID != "s" || r.OSID != "o" || r.CorrelationID != "c" {
		t.Fatalf("reply errado: %+v", r)
	}
}

func TestParse_Erros(t *testing.T) {
	cases := map[string]string{
		"json inválido":   `{`,
		"sem message_id":  `{"type":"x"}`,
		"sem type":        `{"message_id":"1"}`,
		"versão superior": `{"message_id":"1","type":"x","version":99}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(body)); err == nil {
				t.Fatal("esperava erro")
			}
		})
	}
	_, err := Parse([]byte(`{"message_id":"1","type":"x","version":99}`))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("esperava ErrUnsupportedVersion, veio %v", err)
	}
}

func TestDecode_PayloadIncompativel(t *testing.T) {
	env := Envelope{Type: "x", Payload: json.RawMessage(`{"os_code":"não é número"}`)}
	if _, err := Decode[CreateBudget](env); err == nil {
		t.Fatal("esperava erro de tipo")
	}
}
