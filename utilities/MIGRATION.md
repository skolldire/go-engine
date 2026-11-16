# Migration from pkg/utilities helpers/profile

Legacy code and consumers remain unchanged; no alias, bridge or deprecation is added. The new module contains only profile, text, collections and pointers.

Before: app_profile.GetProfileByScope() implicitly reads SCOPE and accepts arbitrary final tokens; predicates disagree about local versus suffix matches. After:

```go
p, err := profile.ParseScope(os.Getenv("SCOPE")) // application owns environment access
if err != nil { return err }
if p.IsLocal() { /* explicit local policy */ }
```

Empty→local; dev-local→local; " API-PROD "→prod; contest/prod-→ErrUnknownProfile. The p.IsX predicates are equality checks on validated profile constants. No file/profile selection policy is introduced into utilities.

| Legacy helper | New destination / migration |
|---|---|
| GetScopeValue/GetProfileByScope/IsXProfile | explicit ParseScope and Profile methods |
| helpers CleanSpaces/Truncate/JoinNonEmpty/RemoveChars | text package |
| byte Truncate | rune limit including suffix, negative→0, invalid bytes repaired by Truncate |
| regexp CleanSpaces | Unicode strings.Fields/Join |
| Keys/Values/Merge/Filter/Map/Find/First/Last | collections, no ordering promise for maps |
| Map(nil) producing [] | new Map(nil) returns nil |
| Filter(non-nil empty/no matches) producing nil | new Filter returns a new nonnil outer slice |
| Find short circuit | new Find visits every element once under D48.G, remembers first match |
| Ptr/ValueOrZero/CoalescePtr | pointers.Ptr/ValueOrZero/FirstNonNil |
| DefaultValue/DefaultDuration/Coalesce | cmp.Or for zero-value fallback |
| DefaultString/CoalesceString | cmp.Or for empty-string fallback; see whitespace difference below |
| Trim/TrimAndCheckEmpty/IsWhitespace/Normalize | strings.TrimSpace and explicit comparison/ToLower |
| Contains/Concat | slices.Contains/slices.Concat |
| IsEmpty*/IsNotEmpty* | len or explicit pointer/value checks |
| GetOrDefault/HasKey | v,ok := m[k] with an explicit fallback branch |
| FirstOrDefault | First then if !ok; do not conflate a present zero with absence |
| NewMap/NewStringMap/NewStringInterfaceMap | make(map[K]V) |
| IsNilOrZero/IsEmptyPtr/ValueOrError | explicit nil/zero policy at the call site |
| WrapError/WrapErrorf/NewError/NewErrorf | nil guard + fmt.Errorf with %w, errors.New or fmt.Errorf |

There is no defaults package. cmp.Or checks Go zero values, not whitespace: legacy DefaultString/CoalesceString treated whitespace-only strings as empty through IsEmptyString. Preserve that policy with an explicit strings.TrimSpace check when needed; do not promise automatic equivalence. Likewise check err!=nil before fmt.Errorf("context: %w",err), since old WrapError(nil,...) returned nil.

Outer copies are shallow, and pointer identity is explicit. No helper clones arbitrary graphs, controls callbacks or adds resource ownership. Root/engine migration and removing old helpers are later tasks.
