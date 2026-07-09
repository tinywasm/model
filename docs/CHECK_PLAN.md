# PLAN — Kind unification (phase A, GATE): `Field.Type` becomes a validating interface, `Field.Widget` deleted

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.
> Phase A of `tinywasm/docs/MASTER_PLAN.md` (Kind unification wave). Everything
> downstream (orm, form, sqlt, postgres, mcp, consumers) waits on this repo.

## Context (zero-context summary)

`tinywasm/model` is the base schema package of the TinyWasm ecosystem: a
`model.Definition` literal is the single source of truth from which `ormc`
generates structs, codecs, and validation. The ecosystem's design doctrine
(CONSTRUCTION_HARNESS.md: typed over `any`, illegal states unrepresentable,
closed by default) is violated by the current `Field` shape:

```go
type Field struct {
    Name   string
    Type   FieldType // storage enum: FieldText, FieldInt, …
    Widget Widget    // OPTIONAL semantic validation + UI binding; nil = neither
    ...
}
```

Problems this plan fixes:

1. **Fail-open validation.** `Field.Validate` only runs semantic checks when
   `Widget != nil`; its own doc says *"Fields without validation rules pass
   any value"*. The zero-value is the unsafe state.
2. **Expressible contradiction.** `{Type: FieldInt, Widget: input.Email()}`
   compiles.
3. **Double declaration.** `Type` + `Widget` are two correlated decisions per
   field; the APIs are written for LLM authors guided by signatures — one
   typed slot means one decision.

## Target design

```go
// Kind replaces the Field.Type enum slot + Field.Widget pair.
// Implementations are stateless templates, safe for concurrent reuse.
type Kind interface {
    Storage() FieldType          // deterministic Go/DDL mapping (see table below)
    Name() string                // semantic name: "text", "int", "email", …
    Validate(value string) error // ALWAYS present — fail-closed
}
```

- `FieldType` (the iota enum `FieldText…FieldRaw`) **stays** as the storage
  mapping, now reached only through `Kind.Storage()`. The deterministic
  table (FieldText/FieldRaw→string, FieldInt→int64, FieldFloat→float64,
  FieldBool→bool, FieldBlob→[]byte, FieldIntSlice→[]int, FieldStruct→Ref
  type, FieldStructSlice→[]Ref type) is unchanged.
- The `Widget` interface and the `Field.Widget` member are **deleted** from
  this package. UI decoration moves entirely to `tinywasm/form/input`
  (phase B): its `input.Input` interface will embed `model.Kind` and own
  `Clone` (render positioning is a form concern, not a schema concern).

**Settled design decisions — do NOT revisit them:**

- **The interface is named `Kind`** (not `Type`, not `Widget`, not
  `FieldKind`). Rationale: `Field.Type Type` would stutter and collide
  mentally with `go/types.Type`; `Widget` connotes UI, which base kinds
  explicitly lack; `FieldKind` is visually confusable with the surviving
  `FieldType` enum. Industry precedent: `protoreflect.Kind` is a field-type
  classification where distinct kinds share one storage type (`sint32` vs
  `int32`) — exactly our `input.Email()` vs `model.Text()` over
  `FieldText`.
- **`NotNull` stays a direct `Field` member** — it does NOT move into
  `Permitted` or into the kind. It is a *presence* contract consumed by
  three layers (DDL `NOT NULL`, codec required-check, form `required`
  attribute), while `Permitted` holds *content* rules; and since `Permitted`
  is an embedded struct, Go composite literals cannot set promoted fields —
  moving it would force `Permitted: model.Permitted{NotNull: true}` in
  every Definition literal, breaking authoring ergonomics. Per-usage
  presence also cannot live in the kind: the same kind serves required and
  optional fields.
- **`Permitted` keeps BOTH of its roles** — it is not dead weight under
  Kind:
  1. *Engine*: kinds implement their validation baselines with it (base
     kinds here; `form/input.Base` already embeds `model.Permitted` and
     delegates to it — that continues).
  2. *Per-field tightening*: the `Field`-embedded `Permitted` stays for
     per-usage constraints. `Minimum`/`Maximum` are genuinely per-column
     (same `Text()` kind, different lengths; `Maximum` drives `VARCHAR(n)`
     DDL in sqlt) and cannot live in the kind.
- **Monotonic composition invariant (MUST be tested):** `Field.Validate`
  order is `NotNull` → `Kind.Validate` (baseline, unconditional) →
  `Permitted` length → `Permitted` chars. Per-field `Permitted` can only
  TIGHTEN — the kind's rejection is final; `Extra` cannot re-allow a
  character the kind's whitelist rejects. To allow more than the kind's
  baseline, the author must choose a different kind (custom kind or
  `Raw()`), never loosen via `Permitted`. Document this in the doc comment
  and add an explicit test (field with `Extra: "<"` over `Text()` still
  rejects `<`).
- Authoring guidance for docs: a char-rule combination repeated across
  fields should become a named custom kind (with its `//ormc:storage`
  directive), not copy-pasted `Permitted` blocks.

## Stage 1 — `Kind` interface + base kinds

New file `kind.go`:

- Define `Kind` exactly as above.
- One unexported implementation + exported constructors covering every
  `FieldType`:

  ```go
  func Text() Kind        // FieldText  — default whitelist EXCLUDES HTML-dangerous chars (<>"'&`)
  func Int() Kind         // FieldInt   — digits (leading '-' allowed)
  func Float() Kind       // FieldFloat — digits, one '.', leading '-'
  func Bool() Kind        // FieldBool  — accepts "true","false","1","0",""
  func Blob() Kind        // FieldBlob  — no content validation (binary; explicit escape hatch)
  func Raw() Kind         // FieldRaw   — no content validation (pre-serialized JSON; explicit escape hatch)
  func Struct() Kind      // FieldStruct      — no string validation (nested Fielder validates itself)
  func IntSlice() Kind    // FieldIntSlice    — no string validation
  func StructSlice() Kind // FieldStructSlice — no string validation
  ```

- `Text()`'s whitelist is the **input-boundary XSS floor**: reuse the
  existing `Permitted` machinery (letters, numbers, spaces, common
  punctuation; never `<`, `>`, `"`, `'`, `&`, backtick). The per-field
  embedded `Permitted` remains available to *tighten* rules; the kind
  provides the baseline. `Raw()` and `Blob()` are the only kinds without a
  content check — that is deliberate: `grep -rn "model.Raw()"` must yield the
  complete audit list of unvalidated inputs.
- `Name()` returns the existing `fieldTypeNames` strings for base kinds
  ("text", "int", …) so `FieldType.String()` semantics survive.

## Stage 2 — `Field` surgery

In `field.go`:

- `Type FieldType` → `Type Kind`.
- Delete `Widget Widget` and the `Widget` interface in `interface.go`
  (keep `Fielder`, `Validator`, `ModuleNaming`, `Model`, `SafeFields`,
  `FielderSlice` untouched).
- `Field.Validate`:
  - keep NotNull-first and empty-value semantics exactly as today;
  - replace the `if f.Widget != nil { f.Widget.Validate }` block with an
    **unconditional** `f.Type.Validate(value)`;
  - a `nil` `Type` (zero-value Field literal) returns a typed error
    (`fmt.Err(f.Name, "kind required")`) — never silently passes. ormc
    (phase B) additionally rejects it at generation time, which is the
    primary guard; the runtime error is the backstop.
  - per-field `Permitted`/length checks run after the kind check, as today.
- Every internal `switch f.Type` / `f.Type ==` site (`ValidateFields`,
  `IsZeroPtr`, `ReadValues`, codec helpers, …) becomes
  `f.Type.Storage()`. Grep the whole module for `\.Type` and migrate each.
- `Definition` doc comment (definition.go line ~15) still mentions widgets —
  rewrite to describe Kinds.

## Stage 3 — tests

These are this package's OWASP-relevant tests — after this wave `model` is
the ecosystem's input-validation boundary, so it owns two OWASP items and no
more: **A03 Injection/XSS** (the `Text()` whitelist is the input-boundary
XSS floor for every consumer) and **A04 Insecure Design** (fail-closed
validation). Tag each test with a short comment naming its item, mirroring
the convention in `tinywasm/user`'s OWASP suite. Auth-related items
(A01/A02/A07/A09) do NOT belong here — they live in `tinywasm/user`, which
owns sessions/JWT/RBAC. Output encoding is the render layer's job, not
this package's.

- Every base kind: `Storage()`/`Name()` round-trip against the enum;
  `Validate` accept/reject cases (Text rejects `<script>`, `"` and backtick;
  Int rejects "12a"; Bool accepts the four forms; Raw/Blob accept anything).
- Fail-closed: a `Field{Name: "x", Type: Text()}` with no per-field rules
  **rejects** HTML-dangerous input (this is the regression test for the old
  `Widget: nil` hole).
- Nil-kind backstop: `Field{Name: "x"}.Validate("v")` errors.
- `ValidateFields`/codec behavior unchanged for a Definition using base
  kinds (adapt existing tests mechanically — assertion semantics must not
  weaken).
- All green under `gotest ./...` (native + wasm).

## Stage 4 — documentation (the WHY must survive this plan)

This `PLAN.md` is deleted when the codejob loop closes. Every design
rationale stated here MUST be migrated into the permanent docs — a reader
(or agent) touching this library later must find the *why*, not just the
rules, or they will "fix" the design back to fail-open.

- `README.md` §"Defining a Model Definition": one `Type:` slot; the
  before/after pair from the master plan; the Raw/Blob audit rule; plus a
  short "Why fail-closed" paragraph: validation used to be opt-in
  (`Widget: nil` = none — the zero-value was the unsafe state), which
  contradicts the ecosystem's CONSTRUCTION_HARNESS doctrine ("closed by
  default"; "a resource left reachable because nobody said otherwise is a
  silent failure"). Link to `docs/ARCHITECTURE.md` for the full rationale.
- `docs/API_FIELD.md`: `Kind` contract, base kinds table (constructor →
  Storage → validation baseline), deleted `Widget`.
- `docs/ARCHITECTURE.md` — new "Validation design" section carrying ALL the
  settled decisions from this plan, with their justifications:
  - why `Kind` replaces the `Type`+`Widget` pair (fail-open default,
    expressible contradiction, two correlated decisions per field);
  - why the interface is named `Kind` (protoreflect.Kind precedent: distinct
    kinds sharing one storage; `Type Type` stutter; `Widget` connotes UI;
    `FieldKind` confusable with `FieldType`);
  - why `NotNull` is a direct `Field` member (presence vs content; consumed
    by DDL/codec/form; Go composite literals cannot set promoted embedded
    fields);
  - `Permitted`'s two roles (engine inside kinds; per-field tightening) and
    the monotonic composition invariant (`NotNull` → Kind → Permitted; the
    field can only tighten — to allow more, pick another kind, never
    loosen);
  - this package's OWASP scope (A03 input-boundary XSS floor via `Text()`
    whitelist, A04 fail-closed by design) and why auth items live in
    `tinywasm/user`, output encoding in the render layer;
  - the authoring rule: repeated char-rule combos become named custom kinds
    (`//ormc:storage` directive), not copy-pasted `Permitted` blocks.

## Harness checklist (mandatory)

- No stdlib: this package compiles for WASM — `tinywasm/fmt` only (it already
  is; keep it that way).
- No `any`/`map` in the public API; constructors return `Kind`, not concrete
  types.
- No string literals in logic: kind names come from the existing
  `fieldTypeNames` table; error verbs are typed via `fmt.Err`.
- Breaking change: bump to the next minor version (v0.0.6 → v0.1.0). Do NOT
  keep deprecated `Widget` shims — downstream migrates in the same wave.
- No unrelated refactors: `Permitted`, `Fielder`, codec interfaces, CRUD
  action bytes stay as-is.

## Acceptance criteria

1. `Field{Type: Text()}` with no other configuration rejects
   HTML-dangerous characters — the fail-open default is gone.
2. The old contradiction is unrepresentable: there is no second slot to
   disagree with `Type`.
3. `grep -rn "Widget" *.go` in this repo → empty (interface, member, and
   mentions in docs comments all gone).
4. `gotest ./...` green (native + wasm); coverage includes every base kind's
   accept/reject table.
5. README/API_FIELD/ARCHITECTURE updated as specified — including the
   "Validation design" rationale section (every settled decision from this
   plan survives in permanent docs; the plan file itself will be deleted).
   Published as the phase-A gate version for the wave.

## Stages

| Stage | File(s) | Action |
|---|---|---|
| 1 | `kind.go` (new) | `Kind` interface + 9 base kind constructors with validation baselines |
| 2 | `field.go`, `interface.go`, `definition.go`, `codec.go` | `Type Kind`, delete `Widget`, fail-closed `Validate`, `.Storage()` at every switch |
| 3 | `*_test.go` | kind tables, fail-closed regression, nil-kind backstop |
| 4 | `README.md`, `docs/API_FIELD.md`, `docs/ARCHITECTURE.md` | new authoring rule |
