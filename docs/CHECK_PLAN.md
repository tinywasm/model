# PLAN — Kind unification follow-up: `Struct`/`StructSlice` take their `*Definition` (close the composition gap)

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Follow-up to the phase-A Kind unification delivery (see the wave's
> orchestrator in `tinywasm/docs/MASTER_PLAN.md`). Small, self-contained.

## Context (zero-context summary)

The Kind unification delivery replaced `Field.Type` (enum) + `Field.Widget`
with a single validating interface:

```go
type Kind interface {
    Storage() FieldType
    Name() string
    Validate(value string) error
}
```

`kind.go` ships base kinds; two of them are **composition kinds**:

```go
func Struct() Kind       // storage FieldStruct
func StructSlice() Kind  // storage FieldStructSlice
```

**The gap:** a composition field is meaningless without knowing WHICH
Definition it nests — ormc derives the generated Go field type from it. That
reference lives today in a separate slot, `Field.Ref *Definition`, so this
compiles and is only caught (at best) at generation time:

```go
{Name: "address", Type: model.Struct()}   // no Ref — what struct is this?
```

This is the exact disease Kind unification cures for `Type`+`Widget`: two
correlated slots whose mismatch/omission the compiler cannot reject.
Doctrine (CONSTRUCTION_HARNESS.md): illegal states unrepresentable; fail at
compile time.

Note `Field.Ref` is currently **overloaded with two meanings** (its own doc
comment says so, disambiguated by Type):
1. composition (Type is FieldStruct/FieldStructSlice): Ref = the nested
   Definition;
2. scalar foreign key (Type is scalar): Ref = the referenced table's
   Definition, driving DDL FK constraints; the Go type stays scalar.

This plan **splits the overload**: meaning 1 moves into the kind
constructor's parameter; meaning 2 stays in `Field.Ref` as its ONLY meaning.

## Stage 1 — parameterized composition kinds

In `kind.go`:

```go
// Struct returns the composition kind nesting the given Definition.
// The ref is REQUIRED: ormc derives the generated Go field type from it.
// No string validation (the nested Fielder validates itself, fail-closed).
func Struct(ref *Definition) Kind

// StructSlice returns the composition kind nesting a slice of the given
// Definition. Same contract as Struct.
func StructSlice(ref *Definition) Kind
```

- Implement via a dedicated unexported kind type (not `baseKind`) that
  stores the ref and exposes it through a new small interface:

  ```go
  // RefKind is implemented by composition kinds (Struct, StructSlice).
  // Consumers (ormc, orm relations) read the nested Definition from here.
  type RefKind interface {
      Kind
      Ref() *Definition
  }
  ```

- `Struct(nil)` / `StructSlice(nil)` cannot be rejected by the compiler —
  guard it: `Validate` returns a typed error (`fmt.Err(name, "ref
  required")`) so the fail-closed backstop catches it, and ormc (already
  planned, phase B) hard-errors at generation.

## Stage 2 — `Field.Ref` keeps ONLY the scalar-FK meaning

In `field.go`:

- Rewrite `Ref`'s doc comment: composition no longer uses it — a scalar
  foreign key is its single meaning (`Type: model.Int(), Ref: &StaffModel`
  → DDL FK; Go type stays the scalar mapping). Delete the two-meanings
  disambiguation text; keep the initialization-cycle caveat (it applies to
  the kind parameter identically: two package-level Definitions cannot
  reference each other in literals — Go rejects the cycle).
- Setting `Field.Ref` on a field whose kind implements `RefKind` is a
  contradiction. `model` cannot reject it at compile time; document it as
  invalid and leave the hard error to ormc generation (phase B plan already
  gets this rule — see Coordination below).

## Stage 3 — tests

- `Struct(&SomeModel)` / `StructSlice(&SomeModel)`: `Storage()`/`Name()`
  round-trip; `RefKind` assertion returns the same `*Definition`.
- Nil-ref backstop: `Struct(nil).Validate("x")` errors; a `Field` with that
  kind fails `Field.Validate`.
- Existing kind tests unaffected; `gotest ./...` green (native + wasm).

## Stage 4 — documentation

- `docs/API_FIELD.md`: update the base kinds table (`Struct(ref)`,
  `StructSlice(ref)`, `RefKind`); update `Ref`'s row to single-meaning FK.
- `docs/ARCHITECTURE.md` §"Validation design": add the settled decision —
  composition ref is a REQUIRED kind parameter (compile-visible), scalar FK
  stays in `Field.Ref` (optional relational metadata); why: the overloaded
  two-meaning `Ref` disambiguated by Type was the same correlated-slots
  disease as `Type`+`Widget`.
- `README.md` Definition example: show one composition field
  (`Type: model.Struct(&AddressModel)`) and one scalar FK
  (`Type: model.Int(), Ref: &StaffModel`).

## Coordination (informative — do not touch other repos)

- The phase-B ormc plan (`tinywasm/orm/docs/PLAN.md`, pending dispatch)
  resolves composition refs from the kind constructor argument and emits
  `Ref:` into the **generated** `Schema()`, so runtime consumers (orm
  relations, sqlt/postgres DDL) keep reading `f.Ref` unchanged. Hand-written
  `Ref:` alongside a `RefKind` = generation error there.
- If anything in this contract blocks that plan, STOP and report.

## Harness checklist (mandatory)

- No stdlib (`tinywasm/fmt` only); no `any`/`map` in public API.
- Typed errors via `fmt.Err`; no new string literals in logic.
- Breaking change over the just-published phase-A version: next patch/minor
  per gopush convention. No deprecated no-arg shims.
- No unrelated refactors: the other seven base kinds, `Field.Validate`
  ordering, and `Permitted` stay exactly as delivered.

## Acceptance criteria

1. `Struct()`/`StructSlice()` without argument no longer compile anywhere in
   the ecosystem — the composition reference is compile-visible.
2. `RefKind` exposes the ref; nil ref fails validation (backstop) and is
   documented as a generation error for ormc.
3. `Field.Ref` doc states the single FK meaning; contradiction rule
   documented.
4. `gotest ./...` green; docs updated per Stage 4.

## Stages

| Stage | File(s) | Action |
|---|---|---|
| 1 | `kind.go` | `Struct(ref)`/`StructSlice(ref)`, `RefKind`, nil backstop |
| 2 | `field.go` | `Ref` doc: single scalar-FK meaning |
| 3 | `tests/` | ref round-trip, nil backstop |
| 4 | `docs/API_FIELD.md`, `docs/ARCHITECTURE.md`, `README.md` | settled decision + examples |
