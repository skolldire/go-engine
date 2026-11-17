# Migration

| Legacy identifier | Replacement | Break |
|---|---|---|
| preset/http.Options | httpapi.New | Explicit hosts/listeners/borrowed health instead of engine options; Run owns only HTTP. |

preset/full has no equivalent here: select tools explicitly. The legacy example's Redis integrations remain unmigrated; the new memory example demonstrates HTTP assembly only. Signals stay in main, resources close after Run; operations use a separate handler/listener. Middleware flags are approved preset defaults, not hidden router defaults. No legacy production is modified or removed. Parity promotion awaits the remaining full validation.
