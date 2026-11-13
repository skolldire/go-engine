# config candidate — not released

Proposed config/v0.1.0, pending owner approval at E6. Go 1.27.4.
D34 revised uses mapstructure/v2 v2.5.0 with strict hooks, legacy MatchName and
mapstructure tags; env.Resolver is public. JSON remains strictly bounded and
precise, with the approved Decoder/UseNumber/token replacement for jsontext.

Viper is only in the independent config/viper module. Engine is unchanged.
Experimental config tags are retired before release; see MIGRATION.md.
E6 manifest/authorization and E7 tag consumers are deferred; no stable release
or tag is created. Evidence for this revision: docs/modules/config-revised-validation.md.
