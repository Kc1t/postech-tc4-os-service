package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrUnsupportedVersion indica uma mensagem de versão de contrato maior que a conhecida.
var ErrUnsupportedVersion = errors.New("contracts: versão de envelope não suportada")

// Meta carrega a identificação de negócio e de rastreio que acompanha toda mensagem.
type Meta struct {
	SagaID        string
	OSID          string
	CorrelationID string
	CausationID   string
}

// NewEnvelope monta um envelope com ID novo e serializa o payload.
func NewEnvelope(msgType, producer string, meta Meta, payload any) (Envelope, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("contracts: serializar payload de %s: %w", msgType, err)
	}
	return Envelope{
		MessageID:     uuid.NewString(),
		Type:          msgType,
		Version:       Version,
		SagaID:        meta.SagaID,
		OSID:          meta.OSID,
		CorrelationID: meta.CorrelationID,
		CausationID:   meta.CausationID,
		Producer:      producer,
		OccurredAt:    time.Now().UTC(),
		Payload:       raw,
	}, nil
}

// Reply devolve a Meta para uma mensagem causada por esta: mesma saga, OS e correlação,
// com a causação apontando para o envelope recebido.
func (e Envelope) Reply() Meta {
	return Meta{SagaID: e.SagaID, OSID: e.OSID, CorrelationID: e.CorrelationID, CausationID: e.MessageID}
}

// Marshal serializa o envelope para o corpo da mensagem.
func (e Envelope) Marshal() ([]byte, error) { return json.Marshal(e) }

// Parse lê um envelope do corpo da mensagem e valida os campos obrigatórios.
func Parse(body []byte) (Envelope, error) {
	var e Envelope
	if err := json.Unmarshal(body, &e); err != nil {
		return Envelope{}, fmt.Errorf("contracts: envelope inválido: %w", err)
	}
	if e.MessageID == "" || e.Type == "" {
		return Envelope{}, errors.New("contracts: envelope sem message_id ou type")
	}
	if e.Version > Version {
		return Envelope{}, fmt.Errorf("%w: %d", ErrUnsupportedVersion, e.Version)
	}
	return e, nil
}

// Decode lê o payload do envelope no tipo indicado.
func Decode[T any](e Envelope) (T, error) {
	var v T
	if err := json.Unmarshal(e.Payload, &v); err != nil {
		return v, fmt.Errorf("contracts: payload inválido para %s: %w", e.Type, err)
	}
	return v, nil
}
