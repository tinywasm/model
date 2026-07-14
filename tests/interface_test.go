package model_test

import "github.com/tinywasm/model"

// modelStub mimics ormc's generated output: if this stub stops satisfying model.Model,
// the contract and the generator have drifted apart.
type modelStub struct{ Name string }

func (m *modelStub) ModelName() string                { return "model_stub" }
func (m *modelStub) Schema() []model.Field            { return nil }
func (m *modelStub) Pointers() []any                  { return []any{&m.Name} }
func (m *modelStub) IsNil() bool                      { return m == nil }
func (m *modelStub) EncodeFields(w model.FieldWriter) {}
func (m *modelStub) DecodeFields(r model.FieldReader) {}

var _ model.Model = (*modelStub)(nil)
