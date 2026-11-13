# Migration to logging (unreleased)

The legacy pkg/utilities/logger, Logrus adapters and consumers remain unchanged. No aliases or compatibility bridge are provided. No logging/logrus or separate AdaptLegacy module is planned (D44). Any required wave7 bridge stays temporarily in legacy pkg/utilities/logger and is removed with it. client/logging is the independent backend-neutral middleware.

| Legacy | Explicit replacement | Difference |
|---|---|---|
| logger.Service | *slog.Logger | Native typed attributes, no framework interface |
| NewService normalized Config | DefaultConfig, edit, Validate/New | Literal zeros fail instead of becoming defaults |
| implicit stdout/file/multiwriter | Dependencies.Output | Application owns opening/closing/rotation/synchronization |
| WithFields | Logger.With / WithGroup | Values materialize at derivation, no later payload mutation |
| ContextExtractor | Explicit attributes or later wrapper | No implicit context metadata |
| SetLogLevel | Fixed Config.Level | No mutable level API |
| Error and WrapError | Caller error handling + explicit log decision | No wrapping/relogging returned errors |
| FatalError | Application termination policy | No library os.Exit |
| Regex SanitizeString/Fields | Recursive key redaction | Free text is not scanned; boundary differs |

Before (legacy application-owned setup):

```go
logger := legacy.NewService(legacy.Config{Level: "info"}, nil)
logger.WithField("component", "orders").Info(ctx, "accepted", map[string]any{"count": 1})
```

After (output supplied and owned by the application):

```go
cfg := logging.DefaultConfig()
logger, err := logging.New(cfg, logging.Dependencies{Output: output})
if err != nil { return err }
logger.With("component", "orders").InfoContext(ctx, "accepted", "count", 1)
```

Do not assume equivalent sanitization. Add the application's sensitive field names explicitly; protect secrets before creating free messages, error text, Stringer text or base64 bytes. The new fixed depth/item/node bounds can replace portions of large payloads with markers. Embedded struct fields remain nested rather than encoding/json flattening. JSON marshalers control representation, then structural protection runs on their decoded output. Text output is not a machine interchange format.

No stable tag is declared. Go1.27.4 is required. Version proposal is logging/v0.1.0, pending approval; local v0.0.0 replaces in consumers are development-only.
