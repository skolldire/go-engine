# Migration

Experimental candidate; legacy production and consumers are unchanged. There are no aliases, compatibility bridges or deprecations.

| Before | Explicit replacement |
|---|---|
| http.Error or ad hoc JSON | NewResponder and the approved Problem Details schema |
| CommonApiError Code/Msg/Details map | Caller Classify returning public Problem Status/Code/Title/Details text |
| err.Error published by a handler | Authorized static/public fields; unknown errors map to a generic 500 |
| Inferred business/driver status | Caller errors.Is/errors.As classification |
| Implicit context/request ID | Optional RequestID accessor; no header lookup/generation |
| Logger inside error handling | Application observation of private causes |
| Nil context replaced by Background | ErrNilContext |
| Ignored serialization/write failures | Write returns writer errors or io.ErrShortWrite |
| Repeated responses after failure | One response boundary before committing headers |

Successful response handling returns nil, not the original business error. Invalid classification writes the fixed fallback and returns ErrInvalidProblem; this error can coexist with a writer error. Details is a bounded public string, not the legacy details map. Request ID is body-only. The caller owns routing, recovery, logging and lifecycle. Existing CommonApiError.Error may format private causes; the new responder never invokes it automatically.
