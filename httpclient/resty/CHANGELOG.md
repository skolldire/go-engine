# Changelog

## Experimental (not released)

- D53: resty v2.17.2 adapter over `httpclient.HTTPClient()`, inheriting transport, origin rule, response limit, timeout and redirect policy; no configuration of its own.
- resty retry fixed off and the native client not exposed; non-2xx returned as a response, with `CheckStatus` yielding `*httpclient.StatusError`.
- Proposed v0.1.0 unapproved; no published base or legacy migration.
