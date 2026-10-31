package messaging

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/Kc1t/postech-tc4-os-service/pkg/contracts"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// SNSAPI é o subconjunto do cliente SNS usado pelo publicador.
type SNSAPI interface {
	Publish(ctx context.Context, in *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

// SQSSender é o subconjunto do cliente SQS usado pelo publicador.
type SQSSender interface {
	SendMessage(ctx context.Context, in *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

// AWSPublisher publica em tópicos SNS e filas SQS.
type AWSPublisher struct {
	sns SNSAPI
	sqs SQSSender
}

func NewAWSPublisher(snsClient SNSAPI, sqsClient SQSSender) *AWSPublisher {
	return &AWSPublisher{sns: snsClient, sqs: sqsClient}
}

var errUnknownKind = errors.New("messaging: tipo de destino desconhecido")

func (p *AWSPublisher) Publish(ctx context.Context, dest Destination, env contracts.Envelope) error {
	body, err := env.Marshal()
	if err != nil {
		return err
	}

	attrs := routingAttributes(env)
	if txn := newrelic.FromContext(ctx); txn != nil {
		seg := newrelic.MessageProducerSegment{
			StartTime:       txn.StartSegmentNow(),
			Library:         libraryFor(dest.Kind),
			DestinationType: destinationTypeFor(dest.Kind),
			DestinationName: dest.ShortName(),
		}
		defer seg.End()

		h := http.Header{}
		txn.InsertDistributedTraceHeaders(h)
		for k, v := range traceAttributes(h) {
			attrs[k] = v
		}
	}

	switch dest.Kind {
	case Topic:
		_, err = p.sns.Publish(ctx, &sns.PublishInput{
			TopicArn:          aws.String(dest.Name),
			Message:           aws.String(string(body)),
			MessageAttributes: snsAttributes(attrs),
		})
	case Queue:
		_, err = p.sqs.SendMessage(ctx, &sqs.SendMessageInput{
			QueueUrl:          aws.String(dest.Name),
			MessageBody:       aws.String(string(body)),
			MessageAttributes: sqsAttributes(attrs),
		})
	default:
		return fmt.Errorf("%w: %q", errUnknownKind, dest.Kind)
	}
	if err != nil {
		return fmt.Errorf("messaging: publicar %s em %s: %w", env.Type, dest.ShortName(), err)
	}
	return nil
}

func libraryFor(k Kind) string {
	if k == Topic {
		return "SNS"
	}
	return "SQS"
}

func destinationTypeFor(k Kind) newrelic.MessageDestinationType {
	if k == Topic {
		return newrelic.MessageTopic
	}
	return newrelic.MessageQueue
}

func snsAttributes(attrs map[string]string) map[string]snstypes.MessageAttributeValue {
	out := make(map[string]snstypes.MessageAttributeValue, len(attrs))
	for k, v := range attrs {
		out[k] = snstypes.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(v)}
	}
	return out
}

func sqsAttributes(attrs map[string]string) map[string]sqstypes.MessageAttributeValue {
	out := make(map[string]sqstypes.MessageAttributeValue, len(attrs))
	for k, v := range attrs {
		out[k] = sqstypes.MessageAttributeValue{DataType: aws.String("String"), StringValue: aws.String(v)}
	}
	return out
}
