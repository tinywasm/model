package model

import "testing"

type fakeModule struct{ name string }

func (f fakeModule) ModelName() string { return f.name }

const (
	resCatalog Resource = "service_catalog"
	resInvoice Resource = "invoices"
)

// Lo que este vocabulario promete: nada se concede si nadie lo dijo.
func TestClosedByDefault(t *testing.T) {
	t.Run("un Grant vacío no concede nada", func(t *testing.T) {
		var g Grant
		if g.Matches(resCatalog, Read) {
			t.Error("el zero value concedió acceso: el default debe denegar")
		}
	})

	t.Run("la acción cero no es un permiso", func(t *testing.T) {
		var a Action
		if a.Has(Read) {
			t.Error("una Action vacía concedió lectura")
		}
		// Y preguntar por "ninguna acción" tampoco es una licencia.
		if AllActions.Has(0) {
			t.Error("preguntar por la acción vacía devolvió permiso")
		}
	})

	t.Run("sin grants, denegado", func(t *testing.T) {
		if AnyGrant(nil, resCatalog, Read) {
			t.Error("una política vacía concedió acceso")
		}
	})

	t.Run("un Authorizer nil deniega, no autoriza", func(t *testing.T) {
		if Allowed(nil, "u1", resCatalog, Read) {
			t.Error("la ausencia de respuesta se tomó por permiso")
		}
	})
}

// Las acciones son un CONJUNTO: "puede leer y actualizar" es UN Grant, no dos.
func TestActionsAreASet(t *testing.T) {
	g := Grant{Resource: resCatalog, Actions: Read | Update}

	if !g.Matches(resCatalog, Read) {
		t.Error("no concedió Read")
	}
	if !g.Matches(resCatalog, Update) {
		t.Error("no concedió Update")
	}
	if g.Matches(resCatalog, Delete) {
		t.Error("concedió Delete, que no estaba en el conjunto")
	}
	if g.Matches(resCatalog, Create) {
		t.Error("concedió Create, que no estaba en el conjunto")
	}
}

func TestGrantMatches(t *testing.T) {
	tests := []struct {
		name  string
		grant Grant
		res   Resource
		act   Action
		want  bool
	}{
		{"exacto", Grant{resCatalog, Read}, resCatalog, Read, true},
		{"otro recurso", Grant{resCatalog, Read}, resInvoice, Read, false},
		{"otra acción", Grant{resCatalog, Read}, resCatalog, Delete, false},
		{"comodín de recurso", Grant{Wildcard, Read}, resInvoice, Read, true},
		{"todas las acciones", Grant{resInvoice, AllActions}, resInvoice, Delete, true},
		{"acceso total", Grant{Wildcard, AllActions}, "lo_que_sea", Create, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.grant.Matches(tt.res, tt.act); got != tt.want {
				t.Errorf("Matches(%q,%d) = %v; want %v", tt.res, tt.act, got, tt.want)
			}
		})
	}
}

// El acceso total es MECANISMO: se sabe interpretar, pero jamás se concede solo.
func TestFullAccessIsNeverGrantedImplicitly(t *testing.T) {
	policy := []Grant{{Resource: resCatalog, Actions: Read}}

	if AnyGrant(policy, resInvoice, Delete) {
		t.Error("una política acotada concedió acceso total: el comodín se coló solo")
	}
	if !AnyGrant(policy, resCatalog, Read) {
		t.Error("la política declarada no concedió lo que sí declaraba")
	}
}

// La razón de que el vocabulario viva junto a ModuleNaming: la identidad de un módulo y el
// recurso que lo protege son EL MISMO nombre, y ahora no pueden divergir.
func TestResourceOfIsTheModuleIdentity(t *testing.T) {
	mod := fakeModule{name: "service_catalog"}

	if got := ResourceOf(mod); got != resCatalog {
		t.Errorf("ResourceOf = %q; se esperaba la identidad del módulo", got)
	}

	// La UI filtra por identidad; el servidor exige por recurso. Si fueran dos strings
	// distintos, al usuario se le mostraría una sección y luego se le negarían sus datos,
	// sin un solo error.
	policy := []Grant{{Resource: ResourceOf(mod), Actions: Read}}
	if !AnyGrant(policy, ResourceOf(mod), Read) {
		t.Error("identidad y recurso divergieron")
	}
}

// El tipo es numérico porque SOLO un tipo numérico cierra el conjunto de verbos: con un
// `type Action string`, `Requires("orders", "write")` sigue compilando — un verbo inventado
// que nadie enforza (el bug real que había en el test de router).
//
// Pero un 6 en una columna de la base de datos es ilegible. Por eso lo que se GUARDA y se
// LOGUEA son las letras de siempre. Representación y almacenamiento son dos preguntas
// distintas, y este es el único sitio donde se encuentran.
func TestActionRoundTripsAsLetters(t *testing.T) {
	tests := []struct {
		action Action
		text   string
	}{
		{0, ""},
		{Read, "r"},
		{Create, "c"},
		{Read | Update, "ru"},
		{AllActions, "crud"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			if got := tt.action.String(); got != tt.text {
				t.Errorf("String() = %q; se esperaba %q — la base de datos debe ser legible", got, tt.text)
			}
			back, err := ParseAction(tt.text)
			if err != nil {
				t.Fatalf("ParseAction(%q): %v", tt.text, err)
			}
			if back != tt.action {
				t.Errorf("ida y vuelta rota: %q → %d; se esperaba %d", tt.text, back, tt.action)
			}
		})
	}
}

func TestParseActionIgnoresOrder(t *testing.T) {
	a, err := ParseAction("ur")
	if err != nil {
		t.Fatal(err)
	}
	if a != Read|Update {
		t.Errorf("ParseAction(\"ur\") = %d; el orden de las letras no debe importar", a)
	}
}

// Una letra desconocida falla RUIDOSAMENTE. Si devolviera cero en silencio, una fila de
// permisos corrupta ("raed") se leería como "sin permisos" y podría re-guardarse así,
// borrando el permiso de verdad sin que nadie viera un error.
func TestParseActionRejectsUnknownVerb(t *testing.T) {
	if _, err := ParseAction("raed"); err == nil {
		t.Fatal("una letra desconocida se tragó en silencio")
	}
	if _, err := ParseAction("x"); err == nil {
		t.Fatal("se aceptó un verbo que no existe")
	}
}

func TestAllowedDelegates(t *testing.T) {
	var gotUser string
	auth := Authorizer(func(userID string, r Resource, a Action) bool {
		gotUser = userID
		return r == resInvoice && a == Read
	})

	if !Allowed(auth, "u1", resInvoice, Read) {
		t.Error("Allowed no delegó la respuesta afirmativa")
	}
	if gotUser != "u1" {
		t.Errorf("userID = %q; want u1", gotUser)
	}
	if Allowed(auth, "u1", resInvoice, Delete) {
		t.Error("Allowed concedió lo que el Authorizer negó")
	}
}
