package model

import "testing"

type fakeModule struct{ name string }

func (f fakeModule) ModelName() string { return f.name }

// Lo que este vocabulario promete: nada se concede si nadie lo dijo.
func TestClosedByDefault(t *testing.T) {
	t.Run("un Grant vacío no concede nada", func(t *testing.T) {
		var g Grant
		if g.Matches("service_catalog", "read") {
			t.Error("el zero value concedió acceso: el default debe denegar")
		}
	})

	t.Run("sin grants, denegado", func(t *testing.T) {
		if AnyGrant(nil, "service_catalog", "read") {
			t.Error("una política vacía concedió acceso")
		}
	})

	t.Run("un Authorizer nil deniega, no autoriza", func(t *testing.T) {
		if Allowed(nil, "u1", "service_catalog", "read") {
			t.Error("la ausencia de respuesta se tomó por permiso")
		}
	})
}

func TestGrantMatches(t *testing.T) {
	tests := []struct {
		name  string
		grant Grant
		res   Resource
		act   Action
		want  bool
	}{
		{"exacto", Grant{"service_catalog", "read"}, "service_catalog", "read", true},
		{"otro recurso", Grant{"service_catalog", "read"}, "invoices", "read", false},
		{"otra acción", Grant{"service_catalog", "read"}, "service_catalog", "write", false},
		{"comodín de recurso", Grant{Wildcard, "read"}, "invoices", "read", true},
		{"comodín de acción", Grant{"invoices", WildcardAction}, "invoices", "delete", true},
		{"comodín total", Grant{Wildcard, WildcardAction}, "lo_que_sea", "lo_que_sea", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.grant.Matches(tt.res, tt.act); got != tt.want {
				t.Errorf("Grant%v.Matches(%q,%q) = %v; want %v", tt.grant, tt.res, tt.act, got, tt.want)
			}
		})
	}
}

// El comodín es MECANISMO: se sabe interpretar, pero jamás se concede solo.
func TestWildcardIsNeverGrantedImplicitly(t *testing.T) {
	policy := []Grant{{Resource: "service_catalog", Actions: "read"}}

	if AnyGrant(policy, "invoices", "delete") {
		t.Error("una política acotada concedió acceso total: el comodín se coló solo")
	}
	if !AnyGrant(policy, "service_catalog", "read") {
		t.Error("la política declarada no concedió lo que sí declaraba")
	}
}

// La razón de que el vocabulario viva junto a ModuleNaming: la identidad de un módulo y
// el recurso que lo protege son EL MISMO nombre, y ahora no pueden divergir.
func TestResourceOfIsTheModuleIdentity(t *testing.T) {
	mod := fakeModule{name: "service_catalog"}

	if got := ResourceOf(mod); got != Resource("service_catalog") {
		t.Errorf("ResourceOf = %q; se esperaba la identidad del módulo", got)
	}

	// El gating de la UI filtra por identidad; el servidor exige por recurso. Si fueran
	// dos strings distintos, al usuario se le mostraría una sección y luego se le
	// negarían sus datos, sin un solo error.
	policy := []Grant{{Resource: ResourceOf(mod), Actions: "read"}}
	if !AnyGrant(policy, ResourceOf(mod), "read") {
		t.Error("identidad y recurso divergieron")
	}
}

func TestAllowedDelegates(t *testing.T) {
	var gotUser string
	auth := Authorizer(func(userID string, r Resource, a Action) bool {
		gotUser = userID
		return r == "invoices" && a == "read"
	})

	if !Allowed(auth, "u1", "invoices", "read") {
		t.Error("Allowed no delegó la respuesta afirmativa")
	}
	if gotUser != "u1" {
		t.Errorf("userID = %q; want u1", gotUser)
	}
	if Allowed(auth, "u1", "invoices", "delete") {
		t.Error("Allowed concedió lo que el Authorizer negó")
	}
}
