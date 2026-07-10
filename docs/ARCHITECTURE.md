# Arquitectura: el modelo como fuente de verdad

> Arquitectura del refactor coordinado en
> [MODEL_BREAK_REFACTOR.md](../../docs/MODEL_BREAK_REFACTOR.md).
> Documenta la inversión de la generación y la decisión de **dónde vive el valor de una fila**.

---

## 1. El cambio que ya está decidido

Hoy escribimos un struct Go **plano + tags string** y `ormc` **genera** el schema:

```go
// model.go  (hoy: struct plano + tags string frágiles)
// ormc:form
type User struct {
    ID    int    `db:"pk,autoinc"`
    Name  string `input:"required,min=2"`
    Email string `input:"email,required"`
}
```

El canal frágil es el **string**: `input:"email,required"` es texto libre que se mapea por
convención a `input.Email()`. Un typo no lo caza el compilador.

> **Formas confirmadas tras Q&A:** el tipo del plano se llama `model.Definition` (cognado
> inglés/español, no colisiona con el `struct` generado ni con `Schema()`). **No hay paquete
> `field`**: la `Definition` se escribe con literales `model.Field{...}` — exactamente lo que hoy
> genera ormc como `_schemaX`. Los kinds salen de `model` (base, siempre validan) o de
> `tinywasm/form/input` (decorados con UI); ver §8 «Validation design».

Invertimos la flecha: **lo que hoy es generado (`_schemaUser`) pasa a escribirse a mano, todo
tipado**, y de ahí se genera lo demás:

```go
// user_model.go  (mañana: ESTO es la fuente de verdad, todo tipado)
var UserModel = model.Definition{
    Name: "user",
    Fields: model.Fields{
        {Name: "id",    Type: model.Int(),  DB: &model.FieldDB{PK: true, AutoInc: true}},
        {Name: "name",  Type: input.Text(), NotNull: true, Permitted: model.Permitted{Minimum: 2}},
        {Name: "email", Type: input.Email(), NotNull: true}, // input.Email() ES un símbolo → typo = no compila
    },
}
```

Esto ya elimina la fragilidad (arnés cerrado: `input.Email()` es un símbolo, no un string) y la
duplicación de escribir *struct + tags*. Lo confirmado del resto del refactor:

- **Generación:** se mantiene, pero alimentada por **tipos**, no por strings.
- **Rollout:** big-bang por capas (`model` es el ancla).
- **Despacho:** secuencial con gate vía el doc maestro.
- **Kind:** replaces the old `FieldType` enum + `Widget` pair. It is an interface
  providing both the storage mapping (`Storage()`) and the semantic validation baseline
  (`Validate()`). standard Kinds like `Text()`, `Int()` provide an input-boundary
  XSS floor by default. Composition kinds (`Struct`, `StructSlice`) are parameterized
  with the nested `Definition`, eliminating the overloaded `Field.Ref` slot.

---

## 2. La restricción dura que condiciona todo: **sin `reflect`**

El ecosistema prohíbe `reflect` (WASM: binario pequeño, O(1), seguridad en compilación — ver
[WHY_GENERATED_CODE_IS_FREE.md](../../orm/docs/WHY_GENERATED_CODE_IS_FREE.md)). Por eso existe todo el
`model_orm.go` generado: sin reflection, **algo** tiene que enumerar los campos de un struct para
construir `Schema`/`Pointers`/codec. Esa restricción es la que hace que la pregunta siguiente sea
difícil.

---

## 3. La pregunta pendiente

> En una fila concreta, **¿dónde vive el valor `"juan@x.com"` del campo email?**

Hay dos respuestas, y son un fork arquitectónico. Una **genera más código**; la otra usa **un único
modelo para todo el ecosistema**.

---

## 4. Opción A — Struct concreto generado

`model.Definition` + `field.*` son solo la **definición**. `ormc` genera un `type User struct` plano y su
plomería no-reflect. El valor vive en el struct generado.

```go
// a mano (fuente de verdad)
var UserModel = model.Definition{ Name: "user", Fields: model.Fields{
    {Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
    {Name: "name", Type: input.Text()},
    {Name: "email", Type: input.Email()},
}}

// generado por ormc → user_gen.go
type User struct { ID int; Name string; Email string }
func (m *User) Schema() []model.Field { return userSchema }
func (m *User) Pointers() []any       { return []any{&m.ID, &m.Name, &m.Email} }
// + EncodeFields / DecodeFields / ModelName / UserList

// uso
u := &User{}
u.Email = "juan@x.com"   // string plano, acceso directo
s := u.Email
```

### Pros
- **Ergonomía nativa:** `u.Email` es un `string`. Acceso, asignación, `==`, autocompletado y chequeo
  de tipo del *acceso* en compilación. Cero fricción.
- **Zero-reflection real:** `Pointers()` y el codec son código concreto, exactamente como hoy.
- **Zero-alloc en hot path:** scan (`rows.Scan(m.Pointers()...)`) y encode no boxean valores; el
  schema es una variable a nivel de paquete (ver [WHY_PACKAGE_LEVEL_SCHEMA.md](../../orm/docs/WHY_PACKAGE_LEVEL_SCHEMA.md)).
- **Binario mínimo:** DCE elimina lo que no se referencia; lo generado que no usas cuesta **0 bytes**.
- **Consumidores intactos:** orm, json, form ya operan sobre punteros concretos; su modelo de acceso
  no cambia. El refactor toca la *fuente*, no el *hot path*.
- **Tipos Go concretos** (`int`, `time.Time`, `[]byte`) se preservan sin conversiones en cada borde.

### Contras
- Sigue habiendo **dos artefactos**: la definición (`UserModel`) y el struct generado (`User`). Es
  menos duplicación que hoy (ya no escribes ambos a mano), pero `user_gen.go` sigue existiendo.
- Requiere el **paso de generación** (watcher / codegen), como hoy.
- La definición-valor y el tipo generado son entidades separadas; hay que ligarlas por convención de
  nombre (`UserModel` → `User`).

---

## 5. Opción B — Modelo único auto-descriptivo

No se genera struct concreto. El propio campo (el `model.Field`) **lleva su valor**. El `model.Definition` es a la
vez schema, validación, form, transporte y fila. Para una fila, se clona el modelo.

```go
// a mano — y esto es TODO
var UserModel = model.Definition{ Name: "user", Fields: model.Fields{
    {Name: "id", Type: model.Int(), DB: &model.FieldDB{PK: true, AutoInc: true}},
    {Name: "name", Type: input.Text()},
    {Name: "email", Type: input.Email()},
}}

// uso
u := UserModel.New()            // instancia = clon del modelo
u.Set("email", "juan@x.com")    // canal string de nuevo, o u.Field(emailKey).Set(...)
v, _ := u.Get("email")
```

### Pros
- **Un solo modelo para todo el ecosistema:** la misma estructura sirve para DB, form, JSON y fila.
  No hay `user_gen.go`.
- **Cero codegen:** no hay watcher, no hay desincronización posible entre fuente y generado.
- **Máxima concisión:** definir un modelo = escribir un valor; añadir un campo = una línea.

### Contras
- **Reintroduce el canal string en el ACCESO:** `u.Get("email")` es texto libre → exactamente la
  fragilidad que este refactor quiere eliminar, movida del schema al acceso. (La *definición* es
  tipada, pero el *uso* no.)
- **Boxing y allocs:** sin reflect, guardar el valor en el field implica `any`/interface por campo, y
  enumerar/acceder por nombre implica mapa o slice+índice. Cada `Get/Set/scan` asigna. Contradice el
  principio WASM zero-alloc del hot path.
- **Presión de GC** en el punto más caliente (scan de resultados, encode masivo).
- **Todo el ecosistema cambia de modelo de acceso:** orm (`rows.Scan`), json (encode/decode) tendrían
  que operar sobre el modelo genérico en vez de punteros concretos → más lento y más difícil de
  mantener sin reflection.
- **Los tipos Go se difuminan en `any`:** conversiones en cada borde; se pierde el `==` y el
  autocompletado tipado del valor.
- En el fondo es un `map[string]any` con fachada tipada solo en la definición: elegante al declarar,
  frágil y caro al usar.

---

## 6. Comparativa por dimensión

| Dimensión | A · Struct generado | B · Modelo único |
|---|---|---|
| Fuente de verdad tipada (sin tags string) | ✅ | ✅ |
| Fragilidad en el **acceso** | ✅ ninguna (`u.Email`) | ❌ `Get("email")` string |
| Zero-reflection | ✅ | ⚠️ difícil / parcial |
| Zero-alloc en hot path (scan/encode) | ✅ | ❌ boxing por campo |
| Binario WASM | ✅ mínimo (DCE) | ⚠️ interfaces + boxing |
| Código que escribes a mano | 1 artefacto (`UserModel`) | 1 artefacto (`UserModel`) |
| Código generado en disco | `user_gen.go` | ninguno |
| Paso de codegen / watcher | sí | no |
| Impacto en consumidores (orm/json/form) | bajo (acceso igual) | alto (nuevo modelo de acceso) |
| Autocompletado del valor | ✅ | ❌ |

---

## 7. Recomendación: **Opción A**

Cumple el objetivo real —*fuente de verdad tipada, sin tags string frágiles, menos duplicación*— sin
sacrificar las dos invariantes que dan valor al ecosistema: **zero-reflection** y **zero-alloc en
WASM**.

Clave: en A **tú solo escribes un artefacto a mano** (`UserModel`); el struct es un detalle de
implementación derivado para cumplir el no-reflect. Es decir, A ya te da "un solo modelo que
escribes", y el "código generado extra" es gratis (DCE) y no lo tecleas tú.

La Opción B logra "un solo modelo" en el papel, pero para conseguirlo **reintroduce justo el canal
string y el costo runtime** que el arnés y el diseño WASM buscan eliminar: la fragilidad no
desaparece, se muda del schema al acceso, y encima añade allocs en el hot path.

> Regla de decisión: si "un solo modelo" obliga a `Get("string")` y a boxing, no es una mejora sobre
> A — es mover la fragilidad de sitio y pagar rendimiento por ella.

---

## 8. Validation design (Kind unification)

The ecosystem's design doctrine (typed over `any`, illegal states unrepresentable,
closed by default) was previously violated by the `Field` shape where validation
was opt-in (`Widget: nil` meant no validation).

### Rationale

- **Kind replaces Type+Widget**: This eliminates the "fail-open" default and the
  "expressible contradiction" (e.g., `{Type: FieldInt, Widget: Email()}`). One
  typed slot means one decision.
- **Interface name `Kind`**: Chosen to avoid stutter (`Type Type`) and collision
  with `go/types.Type`. `Widget` connoted UI, while `Kind` correctly describes
  field classification (matching `protoreflect.Kind` precedent).
- **Fail-closed**: Every field must have a `Kind`. `Field.Validate` is
  unconditional. Standard kinds provide the input-boundary XSS floor (A03).
- **NotNull as direct member**: Presence is a different contract than content
  validation. Keeping it on `Field` allows better authoring ergonomics in
  composite literals and is consumed by DDL, codec, and form layers.
- **Permitted's dual role**: It serves as the engine inside Kinds for baseline
  rules and remains on `Field` for per-usage tightening.
- **Monotonic Composition**: Validation follows the order `NotNull` → `Kind` →
  `Permitted`. A field's `Permitted` rules can only TIGHTEN the constraints;
  the Kind's rejection is final.

### OWASP Scope

`tinywasm/model` is the input-validation boundary for the ecosystem:
- **A03: Injection/XSS**: Handled by `Text()` kind's whitelist floor.
- **A04: Insecure Design**: Handled by the fail-closed architecture.

---

## 9. Nota sobre el mecanismo de generación (AST vs importar-y-reflect)

Se planteó si conviene que el generador **importe el paquete y lea `UserModel` por reflect** (en
build-time, donde reflect sí está permitido) en vez de **parsear el AST** como hoy. Conclusión:
**mantener el parseo de AST (`go/ast`)**. Justificación:

1. **`ormc` corre en vivo como watcher** (`NewFileEvent` en `ormc/watch.go`, `ScanModules` al
   arranque). Importar-y-reflect exigiría **compilar el paquete del usuario en cada guardado** —
   inviable en un watcher, y muchas veces el paquete **no compila a mitad de edición** (justo cuando
   necesitas regenerar).
2. **Problema del huevo y la gallina:** el generador produce el código que *hace compilar* el
   paquete. Si para leer la definición hay que compilar primero, no arranca.
3. **AST no necesita build:** lee los archivos fuente directamente, sin dependencias ni ciclos de
   importación entre el generador y el modelo.
4. **Es más robusto que hoy, no menos:** leer un literal `model.Definition{ Fields: []model.Field{
   {Name:"email", Type: model.FieldText, Widget: input.Email()}, ... } }` por AST es leer
   **composite literals con símbolos tipados** (`Type` es una constante real; `Widget` es un
   `CallExpr` a un símbolo real), no *string-split* de tags. El mecanismo es el mismo (AST); lo que
   cambia es que ahora lee símbolos verificables en vez de texto libre.

El mecanismo actual no solo sirve: es el correcto para un generador que vive dentro del tooling.

---

## 9. Consecuencia para los PLAN.md por librería

Si se confirma **Opción A**:

- **model** (ancla): define `model.Definition{Name, Fields}` y `type Fields = []Field`; mantiene
  `Field` and `Kind` (interfaz). Cero dependencias. **No hay paquete `field`**: la `Definition` se
  escribe con literales `model.Field{...}`.
- **widgets**: cualquier `model.Widget`. `tinywasm/form/input` (`input.Text()`, `input.Email()`, …)
  es la fuente básica y **opcional**.
- **orm/ormc:** invierte el generador — lee el literal `model.Definition` por AST y emite el struct
  concreto + `Schema/Pointers/codec/List`.
- **json / form:** consumen el mismo `Fielder`; cambian su *entrada de definición*, no el hot path.
- **postgres:** consume `Schema()` igual; revisar introspección/DDL contra los nuevos Kinds.
