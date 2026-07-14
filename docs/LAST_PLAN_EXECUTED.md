# PLAN (EJECUTADO 2026-07-14, LOCAL) — `model.Model` pasa a ser el contrato completo

> Ejecutado directamente por el mantenedor (LOCAL, sin codejob). Fase A (gate) de la ola CRUD
> Harness: https://github.com/tinywasm/app/blob/main/docs/CRUD_HARNESS_MASTER_PLAN.md

## El problema

`ormc` genera **siempre** las mismas capacidades juntas para todo modelo (`ModelName`,
`Schema`+`Pointers`, `EncodeFields`, `DecodeFields`, `Validate`). Pero `model` las publicaba
como átomos sueltos y solo nombraba dos combinaciones (`Model` = `Fielder`+`ModuleNaming`,
`SafeFields` = `Fielder`+`Validator`). **Nunca se nombró el registro de dominio completo.**

Consecuencia medida aguas abajo: un layout CRUD (`tinywasm/layout/crudview`, en
`veltylabs/mjosefa-cms`) necesita `Fielder` (generar el form), `Encodable` (mandar el registro
por `router.Caller`) y `Decodable` (recibirlo). Como ese tipo no existía, el consumidor se
inventó la intersección en su propio repo — deuda por construcción, tercera reincidencia del
mismo patrón (antes: wrapper `mcpPublic`, wrapper `AuthModule`).

## Decisión

`model.Model` pasa a ser el contrato completo: `Fielder` + `ModuleNaming` + `Encodable` +
`Decodable`. `Validator` NO entra (eje distinto, ya tiene `SafeFields`).

## Cambios ejecutados

| Archivo | Cambio |
|---|---|
| `interface.go` | `Model` ampliado a `Fielder + ModuleNaming + Encodable + Decodable` |
| `tests/interface_test.go` | nuevo — `modelStub` con la forma exacta de la salida de `ormc` + `var _ model.Model = (*modelStub)(nil)` |
| `docs/CODEC_AND_FIELDER.md` | sección "Qué tipo nombrar en una frontera" + tabla |
| `docs/ARCHITECTURE.md` §10 | nueva: decisión, contrato final, regla anti-intersección |

`gotest ./...` verde. Publicado con gopush como v0.0.14.

## Consumidores (planes en sus repos)

| Repo | Plan |
|---|---|
| `tinywasm/form` | `docs/PLAN.md` — `LoadValues` + `New` falla si no vincula ni un input |
| `tinywasm/layout` | `docs/PLAN.md` — `crudview.New(Config)` + test de consumidor |
| `tinywasm/orm`, `ormc`, `user`, `sqlite` | recompilado contra v0.0.14 (código generado, no reescritura) |
| `veltylabs/modules/service_catalog` | `docs/PLAN.md` — el Kind es el widget |
| `veltylabs/mjosefa-cms` | `docs/PLAN.md` — fases C y D, última de la ola |
