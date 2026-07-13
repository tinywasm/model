package model

import "github.com/tinywasm/fmt"

// The access-control vocabulary. It lives here, next to ModuleNaming, because a module's
// IDENTITY and the RESOURCE that protects it are the same name — and if they were two
// strings that merely happened to match, they would drift apart in silence: the UI would
// gate a section by one name while the server enforced another. Nobody would see an
// error; a user would simply be shown a page and then denied its data.
//
// It cannot live in the library that implements authentication (tinywasm/user): that one
// imports mcp, so mcp would have to import user to type its own field, and user already
// imports mcp. A cycle. And a domain module that only wants to say "my resource is
// service_catalog" must not be forced to drag an OAuth stack behind it.
//
// model is already the zero-dependency contract every layer imports, so the vocabulary is
// free to whoever needs it. Nothing here decides WHO may do WHAT: that is policy, and
// policy belongs to the consumer, written in the consumer's own code.

// Access declares who may reach something — a route, a tool, anything an enforcer guards.
//
// It lives here, not in router or mcp, because BOTH already express these same three states
// and would otherwise each invent their own vocabulary for one idea. router encoded them
// implicitly, as combinations of (Public bool, Resource ""), which made an illegal state
// writable: a route could be marked Public AND carry a Requires, and the enforcer silently
// dropped the permission check. One declared value cannot contradict itself.
type Access uint8

const (
	// AccessGuarded (THE ZERO VALUE) requires an identity AND a permission on its Resource.
	//
	// The default is the strictest state on purpose: something that declares nothing must be
	// unreachable, not quietly open to anyone who happens to be logged in. An enforcer must
	// REJECT a guarded thing with no Resource at registration — loudly, at startup — because
	// authorizing against an empty resource denies every call while looking protected.
	AccessGuarded Access = iota

	// AccessAuthenticated requires an identity but checks no Resource. It is for operations on
	// the CALLER themselves ("who am I?"), where authentication already IS the check.
	//
	// Without this state, a `me` tool had to invent a resource ("profile") the app never
	// declared — a hole in the contract turning into policy inside a library.
	AccessAuthenticated

	// AccessPublic is reachable with no identity at all: static assets, a login page.
	// The rarest case, and the one that must be written on purpose.
	AccessPublic
)

// Resource is the thing being protected: "service_catalog", "invoices".
// The consumer declares its own — a library that enforces access must never invent one.
type Resource string

// Action is what may be done to a Resource. It is a CLOSED set — the four persistence
// verbs — and a SET of them: the type is a bit mask, so one Grant can carry several.
//
// Closed on purpose. Resources are open because the app's language lives there
// ("service_catalog", "invoices"); actions are not, because persistence has exactly these
// four verbs and every tool in the ecosystem already declares one of them. Leaving the verb
// open bought nothing and cost a whole class of bugs: with a free-form string, a typo
// ("raed") compiles and shows up as a denial nobody can explain.
//
// A domain verb like "approve" or "export" is NOT a fifth action: it is another resource
// ("orders:approve"), acted upon with these same four. Keep the app's vocabulary in the
// dimension that is open.
//
// The zero value is no action at all, so it grants nothing — closed by default.
type Action uint8

const (
	Create Action = 1 << iota
	Read
	Update
	Delete
)

// AllActions is every verb. This is what "full access" means for the action dimension:
// a value, not a magic "*" that each implementation parses its own way.
const AllActions = Create | Read | Update | Delete

// Has reports whether the set contains every action in want. An empty want is not a
// licence: a zero Action grants nothing.
func (a Action) Has(want Action) bool {
	if a == 0 || want == 0 {
		return false
	}
	return a&want == want
}

// actionLetters maps each verb to the character it is stored and logged as, in CRUD order.
var actionLetters = [...]struct {
	action Action
	letter byte
}{
	{Create, 'c'},
	{Read, 'r'},
	{Update, 'u'},
	{Delete, 'd'},
}

// String renders the set as the CRUD letters: Read -> "r", Read|Update -> "ru",
// AllActions -> "crud". The empty set is "".
//
// The type is a bit mask because only a numeric type CLOSES the verb set: with a string
// type, `Requires("orders", "write")` still compiles — an invented verb nothing enforces,
// which is the bug this vocabulary exists to kill. But a raw 5 in a database column or a
// log line is unreadable, so the STORED and LOGGED form is the letters, not the bits.
//
// Representation and storage are two different questions. This is the one place they meet.
func (a Action) String() string {
	if a == 0 {
		return ""
	}
	out := make([]byte, 0, len(actionLetters))
	for _, al := range actionLetters {
		if a&al.action != 0 {
			out = append(out, al.letter)
		}
	}
	return string(out)
}

// ParseAction reads back what String wrote: "ru" -> Read|Update. Order does not matter.
//
// An unknown letter is an error, never a silent zero: a permission row that says "raed"
// must fail loudly, not quietly grant nothing (or, worse, be re-saved as no permission at
// all). An empty string parses to the empty set, which grants nothing — that is not an
// error, it is the closed default.
func ParseAction(s string) (Action, error) {
	var a Action
	for i := 0; i < len(s); i++ {
		known := false
		for _, al := range actionLetters {
			if s[i] == al.letter {
				a |= al.action
				known = true
				break
			}
		}
		if !known {
			return 0, fmt.Err("unknown action letter:", string(s[i]), "— the verbs are c, r, u, d")
		}
	}
	return a, nil
}

// RoleCode is how a consumer names a role: "admin", "reception", "practitioner".
// The vocabulary belongs to the app. No library may hardcode one.
type RoleCode string

// Wildcard matches every Resource.
//
// The wildcard is MECHANISM (how a permission is matched), never POLICY: a library may
// honour it when checking, but must never grant it on its own. Handing out full access is
// a decision only the consumer can make, and it must cost an explicit, greppable line.
//
// There is no wildcard for actions: that is simply AllActions. A closed set does not need
// an escape hatch — which is exactly why it is closed.
const Wildcard Resource = "*"

// ResourceOf is the resource that protects a module: its own identity.
//
// This is the whole point of putting the vocabulary next to ModuleNaming. The convention
// "a module's ID is its RBAC resource" used to be a line in a document that nothing
// enforced; here it is a function, so the two can no longer disagree.
func ResourceOf(m ModuleNaming) Resource { return Resource(m.ModelName()) }

// Grant is one line of a policy: what a role may do to a resource. Actions is a SET, so
// "may read and update the catalog" is one Grant, not two:
//
//	Grant{Resource: ResourceCatalog, Actions: Read | Update}
//
// The zero value grants nothing — closed by default, like everything here.
type Grant struct {
	Resource Resource
	Actions  Action
}

// Matches reports whether a Grant covers a (resource, action) pair.
//
// This is the ONE place the wildcard is interpreted, so every enforcer agrees on what it
// means. Two implementations quietly disagreeing about that is a security hole nobody would
// ever see.
func (g Grant) Matches(r Resource, a Action) bool {
	if g.Resource == "" || g.Actions == 0 {
		return false // an empty grant grants nothing
	}
	if g.Resource != Wildcard && g.Resource != r {
		return false
	}
	return g.Actions.Has(a)
}

// AnyGrant reports whether any grant covers the pair. An empty policy denies.
func AnyGrant(grants []Grant, r Resource, a Action) bool {
	for _, g := range grants {
		if g.Matches(r, a) {
			return true
		}
	}
	return false
}

// Authorizer answers whether an identity may perform an action on a resource.
//
// It is the single seam between the layer that ENFORCES access (a router, an MCP server)
// and the one that KNOWS it (an auth module). Enforcers depend on this function type,
// never on a concrete implementation.
//
// A nil Authorizer denies: the absence of an answer is not permission.
type Authorizer func(userID string, r Resource, a Action) bool

// Allowed is the safe way to consult an Authorizer: a missing one denies, instead of
// panicking or — far worse — letting the call through.
func Allowed(auth Authorizer, userID string, r Resource, a Action) bool {
	if auth == nil {
		return false
	}
	return auth(userID, r, a)
}
