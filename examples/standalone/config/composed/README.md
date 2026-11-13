# config composed consumer

Run with Go 1.27.4: `GOWORK=off GOPROXY=off go run -mod=readonly .`.
The sole config replacement is `../../../../config`; composed additionally uses
`../../../../concurrency`. These are local candidate checks, not published tags.
No engine/app/provider is initialized. The application injects its filesystem and
lookup, seeds defaults, validates tool configuration and owns tool lifecycle.
