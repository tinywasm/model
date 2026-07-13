# PLAN (EJECUTADO 2026-07-10, LOCAL) — model: la whitelist explícita del Field reemplaza el piso del kind Text()

> Ejecutado directamente por el mantenedor (LOCAL, sin codejob).
> Decisión tomada tras probar el bug con test y descartar la alternativa
> `model.Opaque()` (rechazada: ensuciaba la API con un kind nuevo cuando
> `Permitted` ya expresa la intención).

## Bug (probado con test antes de decidir)

devbrowser (tests reales chromedp): `Field{Type: model.Text(), Permitted:
Permitted{Extra: []rune("#")}}` rechazaba `"#btn"` con "character not
allowed #". Causa: `Field.Validate` corría el piso del kind PRIMERO e
incondicionalmente; el `Permitted` del Field solo podía endurecer, nunca
extender. Campos con contenido interpretado por máquina (selector CSS, JS,
URL, SQL) no tenían constructor legítimo.

Prueba: `tests/kind_permitted_override_test.go` (quedó como regresión).

## Decisión (mantenedor, 2026-07-10)

El piso de `Text()` es un DEFAULT: si el Field declara una whitelist
POSITIVA propia (`Letters`/`Numbers`/`Spaces`/`Extra`/…) y el kind es el
`Text()` base, la whitelist explícita del autor gobierna (reemplaza el piso)
y el autor asume el encoding de salida de lo que permita. Reglas solo
restrictivas (`NotAllowed`, `StartWith`) NO levantan el piso. Kinds
semánticos (email…) y no-text (Int/Float/Bool) validan SIEMPRE.

## Cambios ejecutados

| Archivo | Cambio |
|---|---|
| `field.go` | `Field.Validate`: salta el piso del kind solo si `hasPositiveCharRules() && f.Type.Name() == fieldTypeNames[FieldText]`; helper nuevo `hasPositiveCharRules()` (whitelist positiva, sin `NotAllowed`/`StartWith`); `hasPermittedRules()` reescrito sobre él; doc del contrato en el comentario |
| `tests/kind_permitted_override_test.go` | nuevo — prueba del bug como regresión (selector CSS `#btn`, control de rechazo de lo no listado) |
| `tests/field_test.go` | `TestFieldValidate_MonotonicInvariant` (contrato viejo) → `TestFieldValidate_ExplicitWhitelistReplacesTextFloor` + `TestFieldValidate_FloorKeptWithoutPositiveWhitelist` |
| `docs/ARCHITECTURE.md` §8 | "Monotonic Composition" reemplazado por el contrato nuevo con rationale |

`gotest ./...` verde. Publicado con gopush.

## Consumidores (planes en sus repos)

| Repo | Cambio |
|------|--------|
| `devbrowser` | selector/script/url/filter/value: `Type: model.Text()` + `Permitted` con su charset explícito (su `docs/PLAN.md` etapa 1) |
| `sqlmcp` | campo `SQL`: ídem, whitelist SQL explícita, en su pasada Kind (paso 4 roadmap) |
| `ormc` | NADA — no hay kind nuevo; la tabla builtin de fase B queda como estaba |
