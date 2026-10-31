package logger

import "context"

type correlationKey struct{}

// WithCorrelationID guarda no ctx o identificador de correlação da requisição. Ele segue para o
// campo correlation_id das mensagens publicadas, ligando a chamada HTTP de origem a toda a saga.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, correlationKey{}, id)
}

// CorrelationID devolve o identificador de correlação presente no ctx ("" se não houver).
func CorrelationID(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}
