package model_test

import (
	"testing"

	"github.com/tinywasm/model"
)

func TestDefinition(t *testing.T) {
	// Construct a Definition literal
	var UserModel = model.Definition{
		Name: "user",
		Fields: model.Fields{
			{Name: "id", Type: model.FieldInt, DB: &model.FieldDB{PK: true, AutoInc: true}},
			{Name: "name", Type: model.FieldText, NotNull: true, Permitted: model.Permitted{Minimum: 2}},
			{Name: "email", Type: model.FieldText, NotNull: true},
		},
	}

	// Assert Name and number of fields
	if UserModel.Name != "user" {
		t.Errorf("expected Name 'user', got %q", UserModel.Name)
	}
	if len(UserModel.Fields) != 3 {
		t.Errorf("expected 3 fields, got %d", len(UserModel.Fields))
	}

	// Definition.Field("email") returns the correct field
	f, ok := UserModel.Field("email")
	if !ok {
		t.Error("expected field 'email' to be found")
	}
	if f.Name != "email" {
		t.Errorf("expected field name 'email', got %q", f.Name)
	}

	// Definition.Field("nope") returns false
	_, ok = UserModel.Field("nope")
	if ok {
		t.Error("expected field 'nope' not to be found")
	}

	// Assert that Fields is assignable from a []Field literal without conversion (alias)
	var fields model.Fields = []model.Field{
		{Name: "test", Type: model.FieldText},
	}
	_ = fields

	// Assert that Field{Type: FieldStruct, Ref: &SomeModel} preserves the Ref pointer
	SomeModel := &model.Definition{Name: "some"}
	fStruct := model.Field{Name: "nested", Type: model.FieldStruct, Ref: SomeModel}
	if fStruct.Ref != SomeModel {
		t.Error("Ref pointer not preserved")
	}

	// Assert that the zero-value of Ref is nil
	var fZero model.Field
	if fZero.Ref != nil {
		t.Error("expected zero-value Ref to be nil")
	}
}
