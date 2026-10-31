package messaging

import (
	"context"
	"sync"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
)

// Published é uma mensagem registrada pelo MemoryPublisher.
type Published struct {
	Dest     Destination
	Envelope contracts.Envelope
}

// MemoryPublisher guarda as mensagens publicadas. Usado em testes e no modo local sem AWS.
type MemoryPublisher struct {
	mu   sync.Mutex
	msgs []Published
	Err  error
}

func NewMemoryPublisher() *MemoryPublisher { return &MemoryPublisher{} }

func (p *MemoryPublisher) Publish(_ context.Context, dest Destination, env contracts.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.Err != nil {
		return p.Err
	}
	p.msgs = append(p.msgs, Published{Dest: dest, Envelope: env})
	return nil
}

// Messages devolve uma cópia das mensagens publicadas.
func (p *MemoryPublisher) Messages() []Published {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Published, len(p.msgs))
	copy(out, p.msgs)
	return out
}

// OfType filtra as mensagens publicadas por tipo.
func (p *MemoryPublisher) OfType(msgType string) []Published {
	var out []Published
	for _, m := range p.Messages() {
		if m.Envelope.Type == msgType {
			out = append(out, m)
		}
	}
	return out
}

// Reset limpa as mensagens registradas.
func (p *MemoryPublisher) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = nil
}
