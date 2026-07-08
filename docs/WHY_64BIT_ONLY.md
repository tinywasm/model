# Why the Codec Is 64-bit Only (`int64` / `float64`)

The numeric surface of the codec is deliberately minimal: `FieldInt` maps to `int64` and
`FieldFloat` maps to `float64`. There are no `FieldInt32`, `FieldFloat32`, or unsigned
variants, and there won't be. This document records why, so the question doesn't reopen.

**See also:** [`API_CODEC.md`](API_CODEC.md) for the interfaces, [`API_FIELD.md`](API_FIELD.md)
for the `FieldType` enum.

## Context

The heart of the tinywasm framework compiles with TinyGo to WebAssembly. The codec
(`FieldWriter` / `FieldReader`) is the boundary every model crosses — JSON wire, DB scan,
JS interop. The intuition "wasm is a 32-bit platform, so 32-bit numbers should be cheaper"
was evaluated and rejected. The reasons:

## 1. WebAssembly has native `i64` and `f64` — there is no cost to save

Wasm (since the MVP) includes `i64` and `f64` as native value types. `int64`/`float64`
arithmetic compiles to direct wasm instructions with no emulation. The "avoid 64-bit on a
32-bit target" rule applies to targets like AVR or ARM Cortex-M where TinyGo must emulate
64-bit math — not to wasm. On our primary target, `int64` is free at the CPU level.

## 2. The wire format is JSON — the Go type width changes nothing on the wire

In JSON, `42` occupies the same bytes whether it came from an `int32` or an `int64`: wire
size depends on decimal digits, not type width. And on the JavaScript side every JSON
number is an IEEE-754 double anyway. A hypothetical `FieldInt32` would produce
byte-identical output. Zero gain at the boundary.

## 3. In TinyGo, binary size is dominated by code — and more types means more code

Adding 32-bit field types would widen:

- `FieldWriter`, `FieldReader`, `ArrayWriter`, `ArrayReader` — extra methods in **every**
  implementation (json, orm scan, sqlite, postgres, …). Interface methods are especially
  expensive: TinyGo cannot dead-code-eliminate them when called through the interface.
- Every `switch field.Type` in `ormc` code generation and in each generated `*_orm.go`.
- `ValidateFields` and every downstream type switch.

That is: to "save" 4 bytes of RAM per field, the wasm binary grows — the opposite of the
framework's goal. The RAM saving is irrelevant in practice: models are short-lived
request/args objects, not bulk buffers. Bulk binary data already has its correct type:
`FieldBlob`.

## 4. A single width means a single overflow semantics

With only `int64`/`float64` there is no truncation matrix: `int64` covers IDs, timestamps,
and counters; `float64` covers every measurement. Adding 32-bit variants would force an
overflow policy at every boundary (DB scan, JSON decode, form input) and invite silent
truncation bugs. The storage layer agrees: SQLite integers are 64-bit and PostgreSQL uses
`bigint`.

## Known caveats (documented, not fixed by 32-bit types)

- **The limit that matters is 2⁵³, not 32 vs 64.** JavaScript's
  `Number.MAX_SAFE_INTEGER` is 2⁵³−1. An `int64` ID above that loses precision when it
  passes through a JS client. The mitigation for such IDs is *ID-as-string*
  (`FieldText`), not a narrower integer.
- **`FieldIntSlice` is the pragmatic exception.** It decodes into `[]int` with an
  `elemType(arr.Int(i))` conversion — and under TinyGo/wasm32 `int` is 32-bit, so a
  silent `int64 → int` narrowing already exists there. Keep `FieldIntSlice` for small
  enumerations (role lists, option ids); values beyond 32 bits belong in scalar `int64`
  fields or `FieldBlob`.

## Removal of `Uint`

The codec interfaces (`FieldWriter` and `FieldReader`) originally included `Uint` methods.
These were removed to simplify the API and reduce binary size. The single ecosystem-wide
caller (`binary.Message`'s `uint32` correlation ID) was migrated losslessly to `Int`.
Genuine 64-bit unsigned values (like hashes) that might exceed `int64` should be
represented as `FieldText` (hex) or `FieldBlob` (8 bytes).
