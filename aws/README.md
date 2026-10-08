# go-engine/aws

AWS clients for go-engine: Cognito, SQS, SNS, SES, S3, SSM, DynamoDB, and an observability-aware AWS facade.

```bash
go get github.com/skolldire/go-engine
```

Clients are built from `config/application.yaml` by the provider you register
with `engine.WithProvider(...)`, or by `aws/preset`, which registers every AWS
provider declared in the configuration at once. Multi-instance clients use the
`*_clients` YAML array format and are selected by name in `From(eng, "name")`.

---

## Cognito

Full authentication lifecycle: registration, sign-in, MFA (TOTP + SMS), JWT validation, token refresh, session management.

```yaml
cognito:
  region: "us-east-1"
  user_pool_id: "us-east-1_XXXXXXXXX"
  client_id: "your-client-id"
  client_secret: ""          # optional
  enable_logging: true
  timeout: 30
```

```go
cog := engine.GetCognito()

// Register
_, err := cog.RegisterUser(ctx, cognito.RegisterUserRequest{
    Username: "john", Email: "john@example.com", Password: "Secure1!",
})

// Authenticate
tokens, err := cog.Authenticate(ctx, cognito.AuthenticateRequest{
    Username: "john", Password: "Secure1!",
})
// If MFA is required, err wraps *cognito.MFARequiredError:
if mfaErr, ok := err.(*cognito.MFARequiredError); ok {
    tokens, err = cog.RespondToMFAChallenge(ctx, cognito.MFAChallengeRequest{
        Username: "john", SessionToken: mfaErr.SessionToken,
        MFACode: "123456", ChallengeType: mfaErr.ChallengeType,
    })
}

// Validate token (offline JWKS verification)
claims, err := cog.ValidateToken(ctx, tokens.AccessToken)

// Refresh
newTokens, err := cog.RefreshToken(ctx, cognito.RefreshTokenRequest{
    RefreshToken: tokens.RefreshToken, Username: "john",
})

// Sign out
cog.SignOut(ctx, tokens.AccessToken)
cog.GlobalSignOut(ctx, tokens.AccessToken)

// Group (role) management — UserPoolID is taken from the client config
cog.AddUserToGroup(ctx, "john", "administrador")
cog.RemoveUserFromGroup(ctx, "john", "administrador")
groups, _ := cog.ListGroupsForUser(ctx, "john")

// MFA setup (TOTP)
assoc, _ := cog.AssociateSoftwareToken(ctx, tokens.AccessToken)
// assoc.QRCode → show to user
cog.VerifySoftwareToken(ctx, tokens.AccessToken, "123456", assoc.Session)
cog.SetUserMFAPreference(ctx, tokens.AccessToken, false, true) // enable TOTP
```

**Engine getter:** `engine.GetCognito() cognito.Service`

---

## SQS

```yaml
sqs_clients:
  - orders:
      endpoint: "http://localhost:4566"   # empty in prod
      wait_time: 20
  - notifications:
      endpoint: "http://localhost:4566"
      wait_time: 10
```

```go
import sqsprovider "github.com/skolldire/go-engine/aws/provider/sqs"

// Registered at build time:
//   engine.New(ctx, engine.WithProvider(sqsprovider.New("orders")))
q, err := sqsprovider.From(eng, "orders")
if err != nil {
	return err
}

// Send
msgID, err := q.SendMsj(ctx, queueURL, `{"event":"order.placed"}`, nil)

// Send JSON (marshals automatically)
_, err = q.SendJSON(ctx, queueURL, myStruct, nil)

// Receive + delete
msgs, err := q.ReceiveMsj(ctx, queueURL, 10)
for _, m := range msgs {
    if err := process(m); err == nil {
        q.DeleteMsj(ctx, queueURL, m.ReceiptHandle)
    }
}
```


## SNS

```yaml
sns_clients:
  - alerts:
      endpoint: "http://localhost:4566"
```

```go
import snsprovider "github.com/skolldire/go-engine/aws/provider/sns"

// Registered at build time:
//   engine.New(ctx, engine.WithProvider(snsprovider.New("alerts")))
sns, err := snsprovider.From(eng, "alerts")
if err != nil {
	return err
}
_, err := sns.Publish(ctx, topicARN, `{"message":"server down"}`, nil)
```

---

## SES

```yaml
ses_clients:
  - transactional:
      region: "us-east-1"
      from_email: "noreply@example.com"
```

```go
import sesprovider "github.com/skolldire/go-engine/aws/provider/ses"

// Registered at build time:
//   engine.New(ctx, engine.WithProvider(sesprovider.New("transactional")))
ses, err := sesprovider.From(eng, "transactional")
if err != nil {
	return err
}
err := ses.SendEmail(ctx, "user@example.com", "Welcome", "<h1>Hello</h1>", "Hello")
err = ses.SendTemplatedEmail(ctx, "user@example.com", "welcome-tpl", templateData)
```

---

## S3

```yaml
s3_clients:
  - assets:
      region: "us-east-1"
      bucket: "my-assets"
```

```go
s3c := engine.GetS3ClientByName("assets")
err := s3c.UploadFile(ctx, "path/to/file.pdf", fileBytes, "application/pdf")
data, err := s3c.DownloadFile(ctx, "path/to/file.pdf")
url, err := s3c.GetPresignedURL(ctx, "path/to/file.pdf", 15*time.Minute)
// Direct client→S3 upload (no file routed through the backend):
putURL, err := s3c.GetPresignedPutURL(ctx, "path/to/file.pdf", "application/pdf", 15*time.Minute)
err = s3c.DeleteFile(ctx, "path/to/file.pdf")
```

---

## SSM Parameter Store

```yaml
ssm_clients:
  - config:
      region: "us-east-1"
```

```go
import ssmprovider "github.com/skolldire/go-engine/aws/provider/ssm"

// Registered at build time:
//   engine.New(ctx, engine.WithProvider(ssmprovider.New("config")))
ssm, err := ssmprovider.From(eng, "config")
if err != nil {
	return err
}
value, err := ssm.GetParameter(ctx, "/my-service/db-password", true) // decrypt=true
params, err := ssm.GetParametersByPath(ctx, "/my-service/", true)
```

---

## DynamoDB

```yaml
dynamo_clients:
  - main:
      endpoint: "http://localhost:4566"
      table_prefix: "dev_"
```

```go
import dynamoprovider "github.com/skolldire/go-engine/aws/provider/dynamo"

// Registered at build time:
//   engine.New(ctx, engine.WithProvider(dynamoprovider.New("main")))
ddb, err := dynamoprovider.From(eng, "main")
if err != nil {
	return err
}
err := ddb.PutItem(ctx, "users", item)
result, err := ddb.GetItem(ctx, "users", key)
items, err := ddb.Query(ctx, "users", "gsi-name", "pk-value", "sk-value")
err = ddb.DeleteItem(ctx, "users", key)
```

---

## AWS facade (`awsclient`)

`awsclient.Client` is an observability-aware facade over the AWS SDK. Use it
when you want a uniform `cloud.Client` interface across several AWS services
with logging, tracing and metrics already wired.

It has no provider: it wraps an `aws.Config` rather than a configured service,
so it is constructed directly instead of being resolved from the engine.

```go
import awsclient "github.com/skolldire/go-engine/aws/pkg/integration/aws"

cloudClient := awsclient.New(awsCfg) // awsCfg is an aws.Config from the AWS SDK
resp, err := cloudClient.Do(ctx, &cloud.Request{
    Operation: "sqs.send_message",
    Path:      queueURL,
    Body:      []byte(`{"event":"exam.completed"}`),
})
```

Wrap it with observability middleware:

```go
import "github.com/skolldire/go-engine/pkg/integration/observability"

chain := cloud.Chain(awsclient.New(awsCfg),
    observability.Logging(eng.Logger()),
    observability.Metrics(observability.NewTelemetryMetricsRecorder(eng.Telemetry())),
    observability.Tracing(myTracer),
)
```

---

## Inbound event normalization

```go
import "github.com/skolldire/go-engine/aws/pkg/integration/inbound"

// In a Lambda handler:
req, err := inbound.NormalizeAPIGatewayEvent(&event)
msg, err := inbound.NormalizeSQSEvent(&sqsEvent)
```
