# PLAN — tinywasm/model: tipos ancla del refactor de modelo

> This plan is dispatched via the CodeJob workflow. See skill: **agents-workflow**.

Eres un agente **sin contexto previo** de este proyecto y **solo tienes este repositorio**
(`tinywasm/model`). Este plan es **autocontenido**: contiene todas las reglas y contratos inline. No
enlaza ni depende de archivos de otros repos.

---

## 1. Contexto (qué y por qué)

El ecosistema **invierte** la generación de modelos. Hoy el autor escribe un struct Go + tags string
frágiles y `ormc` genera el schema. Mañana el autor escribe la definición **tipada a mano** y `ormc`
genera el struct concreto. Este PLAN toca **solo `tinywasm/model`**: es el paquete **ancla** que
define los tipos que todos importan. No modifica `ormc` (va en un PLAN posterior).

Forma canónica que habilitamos (escrita por el usuario final, en otro repo):

```go
var UserModel = model.Definition{
    Name: "user",
    Fields: model.Fields{
        {Name: "id",    Type: model.FieldInt,  DB: &model.FieldDB{PK: true, AutoInc: true}},
        {Name: "name",  Type: model.FieldText, NotNull: true, Widget: input.Text(), Permitted: model.Permitted{Minimum: 2}},
        {Name: "email", Type: model.FieldText, NotNull: true, Widget: input.Email()},
    },
}
```

`Widget` **ya es** una interfaz en este paquete; `form/input` (que la implementa) es una fuente
opcional. **No crear** ningún paquete `field`.

---

## 2. Reglas obligatorias (el arnés)

- **Sin librería estándar** en este paquete: usa `github.com/tinywasm/fmt` (nunca `errors`,
  `strconv`, `strings`, `fmt` de stdlib). Objetivo WASM.
- **`model` tiene 0 dependencias del ecosistema**: NO importar `orm`, `form`, `json`, `form/input`.
- **Sin `reflect`**.
- **Tipado sobre `any`**; explícito sobre implícito; superficie mínima exportada.
- Los cambios deben ser **retrocompatibles a nivel de compilación** para el resto del paquete: no
  romper `Field`, `Fielder`, `Validator`, `ValidateFields` existentes. Solo se **añade**.

---

## 3. Entregables

### 3.1 Nuevo archivo `definition.go`

Define el tipo del plano fuente-de-verdad y su alias de lista:

```go
package model

// Fields is the ordered list of field definitions of a model.
// Alias (not a named type) so a []Field literal is assignable without conversion.
type Fields = []Field

// Definition is the hand-written, fully-typed source of truth for a model.
// From a Definition, the ormc generator produces the concrete Go struct and its
// zero-reflection plumbing (Schema/Pointers/EncodeFields/DecodeFields/List).
//
// This inverts the previous flow (struct + string tags → generated schema):
// the schema literal is now authored by hand and everything else is derived.
type Definition struct {
    Name   string // model identity: table name, ModelName(), route key
    Fields Fields // ordered schema; widgets come from any model.Widget (e.g. form/input)
}
```

Añade un método de conveniencia mínimo, útil para consumidores y para el generador:

```go
// Field returns the field with the given name and true, or a zero Field and false.
func (d Definition) Field(name string) (Field, bool) {
    for _, f := range d.Fields {
        if f.Name == name {
            return f, true
        }
    }
    return Field{}, false
}
```

No exportes más superficie que esto.

### 3.2 Añadir `Ref *Definition` a `Field` (en `field.go`)

Al invertir la flecha, ormc deriva el tipo Go de `Field.Type`. Los tipos soportados son EXACTAMENTE
los del enum `FieldType` — **no hay más** — y cada escalar mapea a **UN** solo tipo Go (mapeo fijo, sin
overrides: un `string` de tipo sería un hueco genérico donde el autor/agente podría escribir cualquier
cosa; el arnés lo prohíbe).

El único caso que el tipo abstracto no resuelve por sí solo es un **struct anidado**: hay que saber a
QUÉ modelo apunta. Se resuelve con una **referencia tipada** a la `Definition` anidada (no un string),
verificada por el compilador:

```go
type Field struct {
    Name      string
    Type      FieldType
    NotNull   bool
    OmitEmpty bool
    Widget    Widget
    DB        *FieldDB
    Ref       *Definition // solo para FieldStruct / FieldStructSlice: apunta a la Definition anidada.
                          // Referencia tipada: si el símbolo no existe, NO compila (arnés cerrado).
    Permitted
}
```

Mapeo determinista `Field.Type` → tipo Go (fijo, sin override), a documentar en el doc-comment:

| `Field.Type` | Tipo Go |
|---|---|
| `FieldText`, `FieldRaw` | `string` |
| `FieldInt` | `int64` |
| `FieldFloat` | `float64` |
| `FieldBool` | `bool` |
| `FieldBlob` | `[]byte` |
| `FieldIntSlice` | `[]int` |
| `FieldStruct` | tipo del `Ref` (obligatorio) |
| `FieldStructSlice` | `[]` del tipo del `Ref` (obligatorio) |

> Enteros y floats son de **64 bits** por defecto y sin variantes (consistencia; nada de overrides).

> `model` **no implementa** la resolución de tipo; solo transporta el dato (`Ref`). El mapeo vive en el
> PLAN de `orm/ormc`. Aquí solo se añade el campo `Ref` y su documentación.

### 3.3 Tests `definition_test.go`

Cubre, con la convención de tests del ecosistema (`gotest ./...` debe pasar):

- Construir una `Definition` literal como la de §1 y afirmar `Name`, `len(Fields)`, y que
  `Definition.Field("email")` devuelve el campo correcto y `("nope")` devuelve `false`.
- Afirmar que `Fields` es asignable desde un `[]Field` literal sin conversión (alias).
- Afirmar que `Field{Type: FieldStruct, Ref: &SomeModel}` conserva el puntero `Ref`, y que el
  zero-value de `Ref` es `nil` (los campos escalares no lo usan).

### 3.4 Documentación

- Actualiza [ARCHITECTURE.md](ARCHITECTURE.md) **solo si** algo quedó desalineado (ya describe la
  inversión; no reescribir).
- Actualiza `docs/API_FIELD.md`: añade la sección `Definition` / `Fields` y el campo `Ref` con la
  tabla de mapeo de tipos.
- Añade una entrada en `README.md` del paquete (o índice de docs) apuntando a `Definition`.

---

## 4. Fuera de alcance (NO hacer)

- No tocar `ormc` ni generar structs (otro repo, PLAN posterior).
- No crear paquete `field` ni constructores tipo `field.Email()`.
- No modificar `Widget`, `Fielder`, `Validator`, `ValidateFields`.
- No añadir dependencias.

---

## 5. Criterio de aceptación

- `gotest ./...` verde en `tinywasm/model`.
- `Definition`, `Fields`, `Definition.Field`, y `Field.Ref` existen y están documentados.
- El paquete sigue sin importar stdlib prohibida ni paquetes del ecosistema.
- La superficie pública nueva es exactamente: `Definition`, `Fields`, `Definition.Field`, campo
  `Field.Ref`.

---

## 6. Etapas

| # | Etapa | Salida | Criterio |
|---|---|---|---|
| 1 | `definition.go` | `Definition`, `Fields`, `Definition.Field` | compila; sin deps nuevas |
| 2 | `Field.Ref` | campo `*Definition` + doc-comment con tabla de tipos | compila; no rompe consumidores |
| 3 | Tests | `definition_test.go` | `gotest ./...` verde |
| 4 | Docs | `API_FIELD.md` + README | reflejan la nueva API |
