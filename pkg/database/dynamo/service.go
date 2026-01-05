package dynamo

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
)

func (dc *DynamoClient) TableName(name string) string {
	if dc.tablePrefix == "" {
		return name
	}
	return fmt.Sprintf("%s-%s", dc.tablePrefix, name)
}

// NewClient creates a DynamoDB-backed Service configured with the provided AWS configuration and package settings.
// If cfg.Endpoint is set, the client uses that endpoint and logs the connection; the returned client carries the provided logger, logging flag, and table prefix.
// If cfg.WithResilience is true, a resilience service is created and attached to the client.
func NewClient(acf aws.Config, cfg Config, log logger.Service) Service {
	client := dynamodb.NewFromConfig(acf, func(o *dynamodb.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			log.Debug(context.Background(), "connecting to external endpoint", map[string]interface{}{"endpoint": cfg.Endpoint})
		} else {
			log.Debug(context.Background(), "connecting to AWS", nil)
		}
	})

	dc := &DynamoClient{
		client:      client,
		logger:      log,
		logging:     cfg.EnableLogging,
		tablePrefix: cfg.TablePrefix,
	}

	if cfg.WithResilience {
		resilienceService := resilience.NewResilienceService(cfg.Resilience, log)
		dc.resilience = resilienceService
	}

	return dc
}

func (dc *DynamoClient) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := dc.ensureContextWithTimeout(ctx)
	defer cancel()

	logFields := map[string]interface{}{"operation": operationName}

	if dc.resilience != nil {
		if dc.logging {
			dc.logger.Debug(ctx, fmt.Sprintf("starting DynamoDB operation with resilience: %s", operationName), logFields)
		}

		result, err := dc.resilience.Execute(ctx, operation)

		if err != nil && dc.logging {
			dc.logger.Error(ctx, fmt.Errorf("error in DynamoDB operation: %w", err), logFields)
		} else if dc.logging {
			dc.logger.Debug(ctx, fmt.Sprintf("DynamoDB operation completed with resilience: %s", operationName), logFields)
		}

		return result, err
	}

	if dc.logging {
		dc.logger.Debug(ctx, fmt.Sprintf("starting DynamoDB operation: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && dc.logging {
		dc.logger.Error(ctx, err, logFields)
	} else if dc.logging {
		dc.logger.Debug(ctx, fmt.Sprintf("DynamoDB operation completed: %s", operationName), logFields)
	}

	return result, err
}

func (dc *DynamoClient) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); hasDeadline {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, DefaultTimeout)
}

func (dc *DynamoClient) GetItem(ctx context.Context, input *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	result, err := dc.execute(ctx, "GetItem", func() (interface{}, error) {
		return dc.client.GetItem(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.GetItemOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected GetItem result: %w", err)
	}

	if len(output.Item) == 0 {
		return nil, ErrItemNotFound
	}

	return output, nil
}

func (dc *DynamoClient) GetItemTyped(ctx context.Context, tableName string, key map[string]types.AttributeValue, item interface{}, optFns ...func(*dynamodb.Options)) error {
	input := &dynamodb.GetItemInput{
		TableName: aws.String(dc.TableName(tableName)),
		Key:       key,
	}

	output, err := dc.GetItem(ctx, input, optFns...)
	if err != nil {
		return err
	}

	err = attributevalue.UnmarshalMap(output.Item, item)
	if err != nil {
		return dc.logger.WrapError(err, ErrUnmarshal.Error())
	}

	return nil
}

func (dc *DynamoClient) PutItem(ctx context.Context, input *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	result, err := dc.execute(ctx, "PutItem", func() (interface{}, error) {
		return dc.client.PutItem(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.PutItemOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected PutItem result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) PutItemTyped(ctx context.Context, tableName string, item interface{}, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return nil, dc.logger.WrapError(err, ErrMarshal.Error())
	}

	input := &dynamodb.PutItemInput{
		TableName: aws.String(dc.TableName(tableName)),
		Item:      av,
	}

	return dc.PutItem(ctx, input, optFns...)
}

func (dc *DynamoClient) DeleteItem(ctx context.Context, input *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	result, err := dc.execute(ctx, "DeleteItem", func() (interface{}, error) {
		return dc.client.DeleteItem(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.DeleteItemOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected DeleteItem result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) DeleteItemByKey(ctx context.Context, tableName string, key map[string]types.AttributeValue, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(dc.TableName(tableName)),
		Key:       key,
	}

	return dc.DeleteItem(ctx, input, optFns...)
}

func (dc *DynamoClient) UpdateItem(ctx context.Context, input *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	result, err := dc.execute(ctx, "UpdateItem", func() (interface{}, error) {
		return dc.client.UpdateItem(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.UpdateItemOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected UpdateItem result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) Query(ctx context.Context, input *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if input.Limit == nil || *input.Limit == 0 {
		input.Limit = aws.Int32(DefaultQueryLimit)
	}

	result, err := dc.execute(ctx, "Query", func() (interface{}, error) {
		return dc.client.Query(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.QueryOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected Query result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) QueryTyped(ctx context.Context, input *dynamodb.QueryInput, items interface{}, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	output, err := dc.Query(ctx, input, optFns...)
	if err != nil {
		return nil, err
	}

	err = attributevalue.UnmarshalListOfMaps(output.Items, items)
	if err != nil {
		return nil, dc.logger.WrapError(err, ErrUnmarshal.Error())
	}

	return output, nil
}

func (dc *DynamoClient) Scan(ctx context.Context, input *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if input.Limit == nil || *input.Limit == 0 {
		input.Limit = aws.Int32(DefaultQueryLimit)
	}

	result, err := dc.execute(ctx, "Scan", func() (interface{}, error) {
		return dc.client.Scan(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.ScanOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected Scan result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) ScanTyped(ctx context.Context, input *dynamodb.ScanInput, items interface{}, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	output, err := dc.Scan(ctx, input, optFns...)
	if err != nil {
		return nil, err
	}

	err = attributevalue.UnmarshalListOfMaps(output.Items, items)
	if err != nil {
		return nil, dc.logger.WrapError(err, ErrUnmarshal.Error())
	}

	return output, nil
}

func (dc *DynamoClient) BatchWriteItem(ctx context.Context, input *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	totalItems := 0
	for _, requests := range input.RequestItems {
		totalItems += len(requests)
	}

	if totalItems > DefaultMaxBatchItems {
		return nil, ErrBatchSizeExceed
	}

	result, err := dc.execute(ctx, "BatchWriteItem", func() (interface{}, error) {
		return dc.client.BatchWriteItem(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.BatchWriteItemOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected BatchWriteItem result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) BatchGetItem(ctx context.Context, input *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	totalItems := 0
	for _, keyAttrs := range input.RequestItems {
		totalItems += len(keyAttrs.Keys)
	}

	if totalItems > DefaultMaxBatchItems {
		return nil, ErrBatchSizeExceed
	}

	result, err := dc.execute(ctx, "BatchGetItem", func() (interface{}, error) {
		return dc.client.BatchGetItem(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.BatchGetItemOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected BatchGetItem result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) TransactWriteItems(ctx context.Context, input *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	if len(input.TransactItems) > DefaultMaxBatchItems {
		return nil, ErrBatchSizeExceed
	}

	result, err := dc.execute(ctx, "TransactWriteItems", func() (interface{}, error) {
		return dc.client.TransactWriteItems(ctx, input, optFns...)
	})

	if err != nil {
		return nil, err
	}

	output, err := client.SafeTypeAssert[*dynamodb.TransactWriteItemsOutput](result)
	if err != nil {
		return nil, fmt.Errorf("unexpected TransactWriteItems result: %w", err)
	}
	return output, nil
}

func (dc *DynamoClient) CreateKeyAttribute(keyName string, value interface{}) (map[string]types.AttributeValue, error) {
	av, err := attributevalue.Marshal(value)
	if err != nil {
		return nil, dc.logger.WrapError(err, ErrMarshal.Error())
	}

	return map[string]types.AttributeValue{
		keyName: av,
	}, nil
}

func (dc *DynamoClient) CreateCompositeKey(partitionKey, partitionValue, sortKey, sortValue interface{}) (map[string]types.AttributeValue, error) {
	pkName, ok := partitionKey.(string)
	if !ok {
		return nil, ErrInvalidKey
	}

	skName, ok := sortKey.(string)
	if !ok {
		return nil, ErrInvalidKey
	}

	pkAV, err := attributevalue.Marshal(partitionValue)
	if err != nil {
		return nil, dc.logger.WrapError(err, ErrMarshal.Error())
	}

	skAV, err := attributevalue.Marshal(sortValue)
	if err != nil {
		return nil, dc.logger.WrapError(err, ErrMarshal.Error())
	}

	return map[string]types.AttributeValue{
		pkName: pkAV,
		skName: skAV,
	}, nil
}