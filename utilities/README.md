# utilities

Pure stdlib-only helpers, Go1.27.4, no require even in tests. Candidate, not published. No environment/file access, commands, mutable caches, goroutines, framework, drivers or dependency on another go-engine module. Caller callback panics propagate.

## profile

ParseScope trims the whole string, lowercases it and validates its last hyphen-separated token: local/test/stage/prod. Empty/whitespace returns Local; an unknown token returns empty Profile and an error wrapping ErrUnknownProfile (the quoted token is intentionally included; it is not treated as a secret). A trailing hyphen has an empty unknown token and fails. String returns the value; IsLocal/IsTest/IsStage/IsProd compare exact constants.

The application chooses the input: `profile.ParseScope(os.Getenv("SCOPE"))`. No package reads SCOPE. dev-local is local; contest is not test; API-PROD is prod after normalization. There is no profile selection cache.

## text

CleanSpaces uses strings.Fields/Join: trim and collapse Unicode whitespace to one ASCII space. JoinNonEmpty preserves the legacy policy of trimming each part and omitting whitespace-only parts. RemoveChars applies sequential literal strings.ReplaceAll removals; empty removal strings have no effect. These functions do not repair invalid UTF-8; original invalid bytes survive unless explicitly selected for removal.

Truncate counts runes like range. A negative maxRunes means0. Output is always valid UTF-8: each invalid input byte becomes U+FFFD, including when the text is short enough not to cut. If input exceeds the limit and maxRunes>3, append "..." within the total rune limit; limits0..3 return only that many initial runes without suffix. Example Truncate("界😊abcd",4) returns "界...". This deliberately replaces legacy byte slicing.

## collections and pointers

Keys/Values return unordered slices; their independent iterations do not promise corresponding index order. Merge returns a new map, last map wins, nil inputs are ignored, no arguments yields a nonnil empty map. Map/Filter return new outer slices; nil input stays nil and a nonnil empty input stays nonnil. First/Last return zero,false on empty input.

D48.G callback contract: Map/Filter/Find visit each element once and in input order on normal traversal. Find remembers the first match but still evaluates later elements; a later callback panic propagates. This differs deliberately from the old short-circuit helper and from slices.IndexFunc. Nil callbacks are not called for empty inputs; calling one for a nonempty input follows ordinary Go panic behavior. Caller callbacks may have their own side effects and concurrent-safety requirements.

Copies are shallow Go value copies, not universal deep clones. The functions retain no internal state/references after returning. Pointer/map/slice fields inside T keep normal Go identity. Ptr returns a pointer to a copy of its argument value; ValueOrZero returns the pointee or zero; FirstNonNil returns the original first nonnil pointer, not a new allocation.

No defaults/errors/checks package is provided. Use cmp.Or, errors.New/fmt.Errorf, len, strings, slices, maps, make and v,ok as appropriate; see MIGRATION.md for whitespace/zero distinctions.

## Verification

`scripts/check-module.sh utilities deps|consumers|quick|stress|fuzz|bench` uses the existing catalog, prefix UTILITIES. Four package suites cover observable behavior, deterministic fuzzing, callback order/panics/nonmutation, stdlib comparisons and measured resource fixtures. Independent minimal uses only utilities+stdlib; composed explicitly reads SCOPE in the application and builds config/viper over an in-memory FS. Neither library imports the other. RELEASE_NOTES.md records actual commands/platforms/results and pending release evidence.
