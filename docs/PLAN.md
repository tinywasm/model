# PLAN — model: named action constants + remove `Uint` from the codec interfaces

> This plan is dispatched via the CodeJob workflow. See skill: agents-workflow.

You are an external agent with **zero prior context** about this project. Everything you
need is in this file. Read it fully before writing code.

---

## Development Rules

- **No standard library for formatting/errors:** this package uses `github.com/tinywasm/fmt`
  (see existing imports in `field.go`). Do not import `errors`, `strings`, or standard `fmt`.
- **No hardcoded values:** repeated literals must be named constants — that is precisely
  what this plan fixes.
- **Non-breaking change only:** the constants MUST be plain `byte` constants. Do **not**
  declare a named type (`type Action byte`) — a named type would break every existing
  `Validate(action byte)` / `ValidateFields(action byte, …)` signature across the ecosystem.
- **Do not run `gopush` or `codejob`** — local developer tools managed outside this task.
  You only modify code, tests, and docs, and run `go test ./...`.

---

## 1. Context and problem

`model` is the root dependency of the tinywasm ecosystem. Its validation entry point is:

```go
// field.go
func ValidateFields(action byte, f Fielder) error
```

and the generated-code contract:

```go
// interface.go
type Validator interface {
    Validate(action byte) error
}
```

The action values are single CRUD bytes — `'c'` (create), `'r'` (read), `'u'` (update),
`'d'` (delete). This convention is shared by the whole ecosystem: `crudp` maps HTTP methods
to these bytes, `mcp` declares `Tool.Action byte` and uses it for RBAC, `form` forwards it
to `ValidateFields`, and `ormc`-generated models implement `Validator`.

**Problem:** the values are magic literals. `field.go` compares `action == 'd'` and
`action == 'c'` inline, tests use raw `'u'`/`'c'`/`'d'`, and every downstream library
re-types the same characters with no single source of truth. This violates the ecosystem
rule that repeated literals must be named constants owned by the defining package.

**Decision (already resolved — do not re-litigate):** keep `byte` as the type (zero-cost
comparisons in WASM/TinyGo hot paths, closed single-character domain, consistency with
roles-as-bytes). Add named **`byte` constants** in `model` as the single source of truth.
The rejected alternative — migrating `action` to `string` — was discarded: it would be an
ecosystem-wide breaking change for negative gain.

---

## 1b. Second problem: `Uint` in the codec interfaces

The codec contract (`codec.go`) declares a `Uint` method in both interfaces:

```go
type FieldWriter interface {
    ...
    Uint(name string, val uint64)   // ← remove
    ...
}
type FieldReader interface {
    ...
    Uint(name string) (uint64, bool) // ← remove
    ...
}
```

This contradicts the documented 64-bit-only design (`docs/WHY_64BIT_ONLY.md`: `int64` and
`float64` are the ONLY numeric types). Evidence it is vestigial:

- No `FieldType` maps to it (there is no `FieldUint`), so `ormc`-generated code can never
  call it.
- It is asymmetric: `ArrayWriter` / `ArrayReader` have no `Uint`.
- Its single caller in the whole ecosystem (`binary.Message`, a hand-written envelope with
  a `uint32` correlation ID) has already been migrated to `Int` in a separate plan — a
  `uint32` fits losslessly in `int64`.
- Every implementation (json, binary, jsvalue) must carry the method, and TinyGo cannot
  dead-code-eliminate interface methods — pure binary-size cost.

Values that genuinely exceed `int64` (e.g. 64-bit hashes) are represented as `FieldText`
(hex) or `FieldBlob` (8 bytes), per `WHY_64BIT_ONLY.md`.

**Decision (already resolved — do not re-litigate):** remove `Uint` from `FieldWriter` and
`FieldReader`. Removing an interface method does not break implementers (extra methods are
legal Go); it only breaks callers, and the last caller is already migrated. Downstream
modules pin their `model` version, so nothing breaks until they choose to update.

---

## 2. Changes

### 2.1 `field.go` — declare the constants

Add immediately above `ValidateFields` (keeping the validation concern in one file):

```go
// CRUD action bytes — the single source of truth for the ecosystem-wide
// action convention used by Validator, ValidateFields, and downstream
// libraries (crudp HTTP mapping, mcp Tool.Action / RBAC, form).
//
// Deliberately plain byte constants (not a named type) so every existing
// `action byte` signature accepts them unchanged.
//
// Not to be confused with orm.Action (an int enum describing query
// execution plans) — that is a different, orm-internal concept.
const (
    ActionCreate byte = 'c'
    ActionRead   byte = 'r'
    ActionUpdate byte = 'u'
    ActionDelete byte = 'd'
)
```

Note: `ActionRead` has no dedicated branch in `ValidateFields` (reads carry no payload to
validate) but is part of the convention — `mcp` tools declare `Action: 'r'` for RBAC. It
belongs in the constant set.

### 2.2 `field.go` — replace internal literals

- `if action == 'd'` → `if action == ActionDelete`
- `if action == 'c' && field.IsPK() && field.IsAutoInc()` → `if action == ActionCreate && …`
- Update the doc comments that currently say `('c', 'u', 'd')` / `'d' delete:` /
  `'c' create:` to reference the constant names (keep the byte values mentioned once in
  the constants' own doc comment).

Grep the whole module for any remaining action literals in non-test code and replace them.

### 2.3 Tests

In `tests/field_test.go`:

- Replace raw action literals (`ValidateFields('u', …)`, `ValidateFields('c', …)`,
  `ValidateFields('d', …)`) with the new constants.
- Add a small contract test pinning the values, since downstream wire/RBAC behavior
  depends on the exact bytes never changing:

  ```go
  func TestActionConstantValues(t *testing.T) {
      if ActionCreate != 'c' || ActionRead != 'r' || ActionUpdate != 'u' || ActionDelete != 'd' {
          t.Fatal("action constants must keep their CRUD byte values — ecosystem wire/RBAC contract")
      }
  }
  ```

- Run `go test ./...` from the module root; all packages must pass.

### 2.4 Docs (action constants)

- `docs/API_FIELD.md`: document the four constants in the `ValidateFields` section
  (values, meaning, and the note that `ActionRead` exists for the RBAC/tool convention).
- `README.md`: in the validation example that calls `model.ValidateFields(action, u)`,
  show usage with a constant (e.g. `model.ValidateFields(model.ActionUpdate, u)`), and
  ensure the docs index still links every file in `docs/`.

### 2.5 `codec.go` — remove `Uint` from both interfaces

- Delete `Uint(name string, val uint64)` from `FieldWriter`.
- Delete `Uint(name string) (uint64, bool)` from `FieldReader`.
- `ArrayWriter` / `ArrayReader` are already `Uint`-free — no change there.

### 2.6 Tests (Uint removal)

In `tests/codec_test.go`, the mock implementations carry the method
(`mockFieldWriter.Uint` ~line 36, `mockFieldReader.Uint` ~line 184). They would still
compile as extra methods, but delete them and any test case exercising them — the module's
own tests must not reference a method the contract no longer has. After the change,
`grep -rn "Uint" .` in this module must only match documentation.

### 2.7 Docs (Uint removal)

- `docs/API_CODEC.md`: remove `Uint` from any interface listing; the interfaces shown must
  match `codec.go` exactly.
- `docs/WHY_64BIT_ONLY.md`: add a short paragraph recording that `Uint` existed in the
  original interfaces and was removed as part of this decision (single caller was
  `binary.Message`'s `uint32` ID, migrated losslessly to `Int`).

---

## 3. Acceptance criteria

1. `ActionCreate`, `ActionRead`, `ActionUpdate`, `ActionDelete` exported as plain `byte`
   constants with the exact values `'c'`, `'r'`, `'u'`, `'d'`.
2. No action byte literals remain in non-test code; tests use the constants plus the
   value-pinning contract test.
3. `Uint` no longer exists anywhere in this module's code (interfaces, mocks, tests) —
   only in documentation, as a recorded removal.
4. Apart from the `Uint` removal, the public API surface is unchanged (no signature or
   type changes).
5. `go test ./...` passes.
6. `docs/API_FIELD.md`, `docs/API_CODEC.md`, `docs/WHY_64BIT_ONLY.md`, and `README.md`
   updated.

---

## 4. Out of scope (do NOT do here)

- Adopting the constants in downstream libraries (`crudp`, `mcp`, `form`, `orm/ormc`,
  `user`). Those are optional follow-up cleanups after this module is published.
- Renaming or changing `orm.Action` (the query-plan enum in the orm module).
- Any change to `ValidateFields` semantics.
- Deleting the now-orphaned `Uint` implementation methods in downstream modules
  (`json`, `jsvalue`, `binary`) — those compile fine as extra methods and are cleaned up
  when each module bumps its `model` dependency.

---

## 5. Stages

| # | Stage | Files | Output |
|---|-------|-------|--------|
| 1 | Declare constants | `field.go` | Four exported `byte` constants with doc comment |
| 2 | Replace internal literals | `field.go` | No magic action bytes in logic or comments |
| 3 | Tests (constants) | `tests/field_test.go` | Constants used; value-pinning contract test |
| 4 | Docs (constants) | `docs/API_FIELD.md`, `README.md` | Constants documented and shown in examples |
| 5 | Remove `Uint` | `codec.go` | Both interfaces without `Uint` |
| 6 | Tests (Uint) | `tests/codec_test.go` | Mock `Uint` methods and cases deleted; `go test ./...` green |
| 7 | Docs (Uint) | `docs/API_CODEC.md`, `docs/WHY_64BIT_ONLY.md` | Listings match `codec.go`; removal recorded |
