package sns

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/skolldire/go-engine/pkg/core/client"
	"github.com/skolldire/go-engine/pkg/utilities/helpers"
	"github.com/skolldire/go-engine/pkg/utilities/logger"
	"github.com/skolldire/go-engine/pkg/utilities/resilience"
	"github.com/skolldire/go-engine/pkg/utilities/validation"
)

func NewClient(acf aws.Config, cfg Config, log logger.Service) Service {
	snsClient := sns.NewFromConfig(acf, func(o *sns.Options) {
		if cfg.BaseEndpoint != "" {
			o.BaseEndpoint = aws.String(cfg.BaseEndpoint)
		}
	})

	cliente := &Cliente{
		cliente: snsClient,
		logger:  log,
		logging: cfg.EnableLogging,
	}

	if cfg.WithResilience {
		cliente.resilience = resilience.NewResilienceService(cfg.Resilience, log)
	}

	if cliente.logging {
		endpoint := cfg.BaseEndpoint
		if endpoint == "" {
			endpoint = "default AWS"
		}
		log.Debug(context.Background(), "SNS client initialized",
			map[string]interface{}{
				"endpoint": endpoint,
			})
	}

	return cliente
}

func (c *Cliente) execute(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	ctx, cancel := c.ensureContextWithTimeout(ctx)
	defer cancel()

	return c.executeOperation(ctx, operationName, operation)
}

func (c *Cliente) ensureContextWithTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		return context.WithTimeout(ctx, DefaultTimeout)
	}
	return context.WithCancel(ctx)
}

func (c *Cliente) executeOperation(ctx context.Context, operationName string, operation func() (interface{}, error)) (interface{}, error) {
	logFields := map[string]interface{}{"operation": operationName, "service": "SNS"}

	if c.resilience != nil {
		return c.executeWithResilience(ctx, operationName, operation, logFields)
	}

	return c.executeWithLogging(ctx, operationName, operation, logFields)
}

func (c *Cliente) executeWithResilience(ctx context.Context, operationName string, operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("starting SNS operation with resilience: %s", operationName), logFields)
	}

	result, err := c.resilience.Execute(ctx, operation)

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("SNS operation completed with resilience: %s", operationName), logFields)
	}

	return result, err
}

func (c *Cliente) executeWithLogging(ctx context.Context, operationName string, operation func() (interface{}, error), logFields map[string]interface{}) (interface{}, error) {
	if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("starting SNS operation: %s", operationName), logFields)
	}

	result, err := operation()

	if err != nil && c.logging {
		c.logger.Error(ctx, err, logFields)
	} else if c.logging {
		c.logger.Debug(ctx, fmt.Sprintf("SNS operation completed: %s", operationName), logFields)
	}

	return result, err
}

func (c *Cliente) CreateTopic(ctx context.Context, nombre string, atributos map[string]string) (string, error) {
	if nombre == "" {
		return "", ErrInvalidInput
	}

	input := &sns.CreateTopicInput{
		Name: aws.String(nombre),
	}

	if len(atributos) > 0 {
		input.Attributes = atributos
	}

	result, err := c.execute(ctx, "CreateTopic", func() (interface{}, error) {
		return c.cliente.CreateTopic(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrCreateTopic.Error())
	}

	response, err := client.SafeTypeAssert[*sns.CreateTopicOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrCreateTopic.Error())
	}
	return *response.TopicArn, nil
}

func (c *Cliente) DeleteTopic(ctx context.Context, arn string) error {
	if arn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "DeleteTopic", func() (interface{}, error) {
		return c.cliente.DeleteTopic(ctx, &sns.DeleteTopicInput{
			TopicArn: aws.String(arn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrDeleteTopic.Error())
	}

	return nil
}

func (c *Cliente) GetTopics(ctx context.Context) ([]string, error) {
	result, err := c.execute(ctx, "GetTopics", func() (interface{}, error) {
		return c.cliente.ListTopics(ctx, &sns.ListTopicsInput{})
	})

	if err != nil {
		return nil, c.logger.WrapError(err, ErrListTopics.Error())
	}

	response, err := client.SafeTypeAssert[*sns.ListTopicsOutput](result)
	if err != nil {
		return nil, c.logger.WrapError(err, ErrListTopics.Error())
	}
	topics := make([]string, len(response.Topics))

	for i, topic := range response.Topics {
		topics[i] = *topic.TopicArn
	}

	return topics, nil
}

func (c *Cliente) PublishMsj(ctx context.Context, temaArn string, mensaje string, atributos map[string]types.MessageAttributeValue) (string, error) {
	if temaArn == "" || mensaje == "" {
		return "", ErrInvalidInput
	}

	input := &sns.PublishInput{
		TopicArn:          aws.String(temaArn),
		Message:           aws.String(mensaje),
		MessageAttributes: atributos,
	}

	result, err := c.execute(ctx, "PublishMsj", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}

	response, err := client.SafeTypeAssert[*sns.PublishOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}
	return *response.MessageId, nil
}

func (c *Cliente) PublishJSON(ctx context.Context, temaArn string, mensaje interface{}, atributos map[string]types.MessageAttributeValue) (string, error) {
	if temaArn == "" || mensaje == nil {
		return "", ErrInvalidInput
	}

	jsonBytes, err := json.Marshal(mensaje)
	if err != nil {
		return "", fmt.Errorf("error converting message to JSON: %w", err)
	}

	input := &sns.PublishInput{
		TopicArn:          aws.String(temaArn),
		Message:           aws.String(string(jsonBytes)),
		MessageStructure:  aws.String("json"),
		MessageAttributes: atributos,
	}

	result, err := c.execute(ctx, "PublishJSON", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}

	response, err := client.SafeTypeAssert[*sns.PublishOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}
	return *response.MessageId, nil
}

func (c *Cliente) CreateSubscription(ctx context.Context, temaArn, protocolo, endpoint string) (string, error) {
	if temaArn == "" || protocolo == "" || endpoint == "" {
		return "", ErrInvalidInput
	}

	input := &sns.SubscribeInput{
		TopicArn:              aws.String(temaArn),
		Protocol:              aws.String(protocolo),
		Endpoint:              aws.String(endpoint),
		ReturnSubscriptionArn: true,
	}

	result, err := c.execute(ctx, "CreateSubscription", func() (interface{}, error) {
		return c.cliente.Subscribe(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrSubscription.Error())
	}

	response, err := client.SafeTypeAssert[*sns.SubscribeOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrSubscription.Error())
	}
	return *response.SubscriptionArn, nil
}

func (c *Cliente) DeleteSubscription(ctx context.Context, suscripcionArn string) error {
	if suscripcionArn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "DeleteSubscription", func() (interface{}, error) {
		return c.cliente.Unsubscribe(ctx, &sns.UnsubscribeInput{
			SubscriptionArn: aws.String(suscripcionArn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrSubscription.Error())
	}

	return nil
}

func (c *Cliente) EnableLogging(activar bool) {
	c.logging = activar
}

func (c *Cliente) SendSMS(ctx context.Context, phoneNumber, message string, attributes map[string]types.MessageAttributeValue) (string, error) {
	if phoneNumber == "" || message == "" {
		return "", ErrInvalidInput
	}

	cleaned := helpers.RemoveChars(helpers.Trim(phoneNumber), " ", "-", "(", ")")
	if err := validation.GetGlobalValidator().Var(cleaned, "required,e164"); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	input := &sns.PublishInput{
		PhoneNumber:       aws.String(phoneNumber),
		Message:           aws.String(message),
		MessageAttributes: attributes,
	}

	result, err := c.execute(ctx, "SendSMS", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrSMSFailed.Error())
	}

	response, err := client.SafeTypeAssert[*sns.PublishOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrSMSFailed.Error())
	}
	return *response.MessageId, nil
}

func (c *Cliente) SendBulkSMS(ctx context.Context, phoneNumbers []string, message string, attributes map[string]types.MessageAttributeValue) (*BulkSMSResult, error) {
	if len(phoneNumbers) == 0 || message == "" {
		return nil, ErrInvalidInput
	}

	result := &BulkSMSResult{
		Successful: make([]SMSResult, 0, len(phoneNumbers)),
		Failed:     make([]SMSResult, 0, len(phoneNumbers)),
	}

	for _, phoneNumber := range phoneNumbers {
		messageID, err := c.SendSMS(ctx, phoneNumber, message, attributes)
		if err != nil {
			result.Failed = append(result.Failed, SMSResult{
				PhoneNumber: phoneNumber,
				Error:       err,
				Status:      "failed",
			})
		} else {
			result.Successful = append(result.Successful, SMSResult{
				PhoneNumber: phoneNumber,
				MessageID:   messageID,
				Status:      "success",
			})
		}
	}

	return result, nil
}

func (c *Cliente) SetSMSAttributes(ctx context.Context, attributes map[string]string) error {
	if len(attributes) == 0 {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "SetSMSAttributes", func() (interface{}, error) {
		return c.cliente.SetSMSAttributes(ctx, &sns.SetSMSAttributesInput{
			Attributes: attributes,
		})
	})

	if err != nil {
		return c.logger.WrapError(err, "error setting SMS attributes")
	}

	return nil
}

func (c *Cliente) GetSMSAttributes(ctx context.Context) (map[string]string, error) {
	result, err := c.execute(ctx, "GetSMSAttributes", func() (interface{}, error) {
		return c.cliente.GetSMSAttributes(ctx, &sns.GetSMSAttributesInput{})
	})

	if err != nil {
		return nil, c.logger.WrapError(err, "error getting SMS attributes")
	}

	response, err := client.SafeTypeAssert[*sns.GetSMSAttributesOutput](result)
	if err != nil {
		return nil, c.logger.WrapError(err, "error getting SMS attributes")
	}
	return response.Attributes, nil
}

func (c *Cliente) CheckPhoneNumberOptedOut(ctx context.Context, phoneNumber string) (bool, error) {
	if phoneNumber == "" {
		return false, ErrInvalidInput
	}

	result, err := c.execute(ctx, "CheckPhoneNumberOptedOut", func() (interface{}, error) {
		return c.cliente.CheckIfPhoneNumberIsOptedOut(ctx, &sns.CheckIfPhoneNumberIsOptedOutInput{
			PhoneNumber: aws.String(phoneNumber),
		})
	})

	if err != nil {
		return false, c.logger.WrapError(err, "error checking phone number opt-out status")
	}

	response, err := client.SafeTypeAssert[*sns.CheckIfPhoneNumberIsOptedOutOutput](result)
	if err != nil {
		return false, c.logger.WrapError(err, "error checking phone number opt-out status")
	}
	return response.IsOptedOut, nil
}

func (c *Cliente) ListOptedOutPhoneNumbers(ctx context.Context) ([]string, error) {
	var allNumbers []string
	var nextToken *string

	for {
		result, err := c.execute(ctx, "ListOptedOutPhoneNumbers", func() (interface{}, error) {
			return c.cliente.ListPhoneNumbersOptedOut(ctx, &sns.ListPhoneNumbersOptedOutInput{
				NextToken: nextToken,
			})
		})

		if err != nil {
			return nil, c.logger.WrapError(err, "error listing opted-out phone numbers")
		}

		response, err := client.SafeTypeAssert[*sns.ListPhoneNumbersOptedOutOutput](result)
		if err != nil {
			return nil, c.logger.WrapError(err, "error listing opted-out phone numbers")
		}
		allNumbers = append(allNumbers, response.PhoneNumbers...)

		if response.NextToken == nil {
			break
		}
		nextToken = response.NextToken
	}

	return allNumbers, nil
}

func (c *Cliente) OptInPhoneNumber(ctx context.Context, phoneNumber string) error {
	if phoneNumber == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "OptInPhoneNumber", func() (interface{}, error) {
		return c.cliente.OptInPhoneNumber(ctx, &sns.OptInPhoneNumberInput{
			PhoneNumber: aws.String(phoneNumber),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, "error opting in phone number")
	}

	return nil
}

func (c *Cliente) CreatePlatformApplication(ctx context.Context, name, platform string, credentials map[string]string) (string, error) {
	if name == "" || platform == "" {
		return "", ErrInvalidInput
	}

	input := &sns.CreatePlatformApplicationInput{
		Name:     aws.String(name),
		Platform: aws.String(platform),
	}

	if len(credentials) > 0 {
		input.Attributes = credentials
	}

	result, err := c.execute(ctx, "CreatePlatformApplication", func() (interface{}, error) {
		return c.cliente.CreatePlatformApplication(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrCreatePlatformApp.Error())
	}

	response, err := client.SafeTypeAssert[*sns.CreatePlatformApplicationOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrCreatePlatformApp.Error())
	}
	return *response.PlatformApplicationArn, nil
}

func (c *Cliente) CreatePlatformEndpoint(ctx context.Context, platformApplicationArn, token string, customUserData string, attributes map[string]string) (string, error) {
	if platformApplicationArn == "" || token == "" {
		return "", ErrInvalidInput
	}

	input := &sns.CreatePlatformEndpointInput{
		PlatformApplicationArn: aws.String(platformApplicationArn),
		Token:                  aws.String(token),
	}

	if customUserData != "" {
		input.CustomUserData = aws.String(customUserData)
	}

	if len(attributes) > 0 {
		input.Attributes = attributes
	}

	result, err := c.execute(ctx, "CreatePlatformEndpoint", func() (interface{}, error) {
		return c.cliente.CreatePlatformEndpoint(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrCreatePlatformEndpoint.Error())
	}

	response, err := client.SafeTypeAssert[*sns.CreatePlatformEndpointOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrCreatePlatformEndpoint.Error())
	}
	return *response.EndpointArn, nil
}

func (c *Cliente) PublishToEndpoint(ctx context.Context, endpointArn string, message string, messageAttributes map[string]types.MessageAttributeValue) (string, error) {
	if endpointArn == "" || message == "" {
		return "", ErrInvalidInput
	}

	input := &sns.PublishInput{
		TargetArn:         aws.String(endpointArn),
		Message:           aws.String(message),
		MessageAttributes: messageAttributes,
	}

	result, err := c.execute(ctx, "PublishToEndpoint", func() (interface{}, error) {
		return c.cliente.Publish(ctx, input)
	})

	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}

	response, err := client.SafeTypeAssert[*sns.PublishOutput](result)
	if err != nil {
		return "", c.logger.WrapError(err, ErrPublication.Error())
	}
	return *response.MessageId, nil
}

func (c *Cliente) SetEndpointAttributes(ctx context.Context, endpointArn string, attributes map[string]string) error {
	if endpointArn == "" || len(attributes) == 0 {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "SetEndpointAttributes", func() (interface{}, error) {
		return c.cliente.SetEndpointAttributes(ctx, &sns.SetEndpointAttributesInput{
			EndpointArn: aws.String(endpointArn),
			Attributes:  attributes,
		})
	})

	if err != nil {
		return c.logger.WrapError(err, "error setting endpoint attributes")
	}

	return nil
}

func (c *Cliente) GetEndpointAttributes(ctx context.Context, endpointArn string) (map[string]string, error) {
	if endpointArn == "" {
		return nil, ErrInvalidInput
	}

	result, err := c.execute(ctx, "GetEndpointAttributes", func() (interface{}, error) {
		return c.cliente.GetEndpointAttributes(ctx, &sns.GetEndpointAttributesInput{
			EndpointArn: aws.String(endpointArn),
		})
	})

	if err != nil {
		return nil, c.logger.WrapError(err, "error getting endpoint attributes")
	}

	response, err := client.SafeTypeAssert[*sns.GetEndpointAttributesOutput](result)
	if err != nil {
		return nil, c.logger.WrapError(err, "error getting endpoint attributes")
	}
	return response.Attributes, nil
}

func (c *Cliente) DeleteEndpoint(ctx context.Context, endpointArn string) error {
	if endpointArn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "DeleteEndpoint", func() (interface{}, error) {
		return c.cliente.DeleteEndpoint(ctx, &sns.DeleteEndpointInput{
			EndpointArn: aws.String(endpointArn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, ErrDeleteEndpoint.Error())
	}

	return nil
}

func (c *Cliente) DeletePlatformApplication(ctx context.Context, platformApplicationArn string) error {
	if platformApplicationArn == "" {
		return ErrInvalidInput
	}

	_, err := c.execute(ctx, "DeletePlatformApplication", func() (interface{}, error) {
		return c.cliente.DeletePlatformApplication(ctx, &sns.DeletePlatformApplicationInput{
			PlatformApplicationArn: aws.String(platformApplicationArn),
		})
	})

	if err != nil {
		return c.logger.WrapError(err, "error deleting platform application")
	}

	return nil
}

func (c *Cliente) ListPlatformApplications(ctx context.Context) ([]PlatformApplication, error) {
	var allApps []PlatformApplication
	var nextToken *string

	for {
		result, err := c.execute(ctx, "ListPlatformApplications", func() (interface{}, error) {
			return c.cliente.ListPlatformApplications(ctx, &sns.ListPlatformApplicationsInput{
				NextToken: nextToken,
			})
		})

		if err != nil {
			return nil, c.logger.WrapError(err, "error listing platform applications")
		}

		response, err := client.SafeTypeAssert[*sns.ListPlatformApplicationsOutput](result)
		if err != nil {
			return nil, c.logger.WrapError(err, "error listing platform applications")
		}
		for _, app := range response.PlatformApplications {
			allApps = append(allApps, PlatformApplication{
				PlatformApplicationArn: aws.ToString(app.PlatformApplicationArn),
				Attributes:             app.Attributes,
			})
		}

		if response.NextToken == nil {
			break
		}
		nextToken = response.NextToken
	}

	return allApps, nil
}

func (c *Cliente) ListEndpointsByPlatformApplication(ctx context.Context, platformApplicationArn string) ([]Endpoint, error) {
	if platformApplicationArn == "" {
		return nil, ErrInvalidInput
	}

	allEndpoints := make([]Endpoint, 0, 100)
	var nextToken *string

	for {
		result, err := c.execute(ctx, "ListEndpointsByPlatformApplication", func() (interface{}, error) {
			return c.cliente.ListEndpointsByPlatformApplication(ctx, &sns.ListEndpointsByPlatformApplicationInput{
				PlatformApplicationArn: aws.String(platformApplicationArn),
				NextToken:              nextToken,
			})
		})

		if err != nil {
			return nil, c.logger.WrapError(err, "error listing endpoints")
		}

		response, err := client.SafeTypeAssert[*sns.ListEndpointsByPlatformApplicationOutput](result)
		if err != nil {
			return nil, c.logger.WrapError(err, "error listing endpoints")
		}
		for _, endpoint := range response.Endpoints {
			allEndpoints = append(allEndpoints, Endpoint{
				EndpointArn: aws.ToString(endpoint.EndpointArn),
				Attributes:  endpoint.Attributes,
			})
		}

		if response.NextToken == nil {
			break
		}
		nextToken = response.NextToken
	}

	return allEndpoints, nil
}
