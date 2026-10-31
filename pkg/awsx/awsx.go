// Package awsx carrega a configuração da AWS SDK v2 com a instrumentação do New Relic.
//
// O endpoint segue a cadeia padrão da SDK: em desenvolvimento, AWS_ENDPOINT_URL aponta para o
// LocalStack; no EKS fica vazio e as chamadas vão para a AWS. Credenciais e região vêm das
// variáveis padrão (AWS_REGION, AWS_ACCESS_KEY_ID, ...).
package awsx

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	nrawssdk "github.com/newrelic/go-agent/v3/integrations/nrawssdk-v2"
)

// LoadConfig devolve a aws.Config com o middleware do New Relic registrado. O middleware é
// inicializado com um contexto sem transação: cada chamada usa a transação do ctx da própria
// chamada, então SNS/SQS/DynamoDB aparecem como segmentos da transação certa.
func LoadConfig(ctx context.Context) (aws.Config, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return aws.Config{}, err
	}
	nrawssdk.InitializeMiddleware(&cfg.APIOptions, context.Background(), cfg.Credentials)
	return cfg, nil
}
