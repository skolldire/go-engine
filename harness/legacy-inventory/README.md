# legacy-inventory

Nonpublished harness (kind `harness`, track `none`) for the parity audit of lot L6. Standard library only; it requires nothing and is required by nothing.

It lists every exported identifier of the legacy go-engine code and checks a parity matrix against that list.

```sh
cd harness/legacy-inventory
GOWORK=off go run . inventory ../..                                    # TSV: module, package, kind, identifier, doc
GOWORK=off go run . check ../.. ../../docs/modularization/parity.md    # exhaustiveness and count per status
```

## What counts as legacy

The scope is derived from `modules.tsv`, not maintained by hand: every module on the `coordinated` track, plus `example`. Nested modules are separate boundaries and are not descended into, which keeps the new standalone modules out of the inventory. Test files, `testdata`, hidden directories and files with " 2" in their name are skipped.

An identifier is an exported constant, variable, type or function, or an exported method of an exported type, named `<package dir>.<Name>` or `<package dir>.<Type>.<Method>`. A name declared in two files under different build tags counts once. The `doc` column is the first sentence of the declaration's comment, bounded to 160 characters, or empty.

## What `check` verifies

A matrix row is a Markdown table row whose first cell is the identifier in backticks and whose fourth cell is one of `MIGRADO`, `MIGRADO CON RUPTURA`, `PLANIFICADO`, `HUECO`, `DESCARTE PROPUESTO`, `DESCARTE APROBADO` (D56). Rows whose identifier starts with `cap:` are capabilities. They have no declaration to compare against and are only counted.

`check` fails (exit 1) when an identifier has no row, has more than one, a row names something that is not an exported legacy identifier, or a status is unknown. It prints the count per status for identifiers and capabilities. It does not judge whether a status is right; that is the matrix's evidence column.

The parity matrix lives in `docs/`, which is git-ignored, so the check runs locally and is not part of CI.
