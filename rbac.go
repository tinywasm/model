package model

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

// Resource is the thing being protected: "service_catalog", "invoices".
// The consumer declares its own — a library that enforces access must never invent one.
type Resource string

// Action is the verb performed on a Resource: "read", "write", "orders:export".
// Readable and extensible on purpose; not a cryptic byte.
type Action string

// RoleCode is how a consumer names a role: "admin", "reception", "practitioner".
// The vocabulary belongs to the app. No library may hardcode one.
type RoleCode string

// Wildcard matches every Resource, and WildcardAction every Action.
//
// The wildcard is MECHANISM (how a permission is matched), never POLICY: a library may
// honour it when checking, but must never grant it on its own. Handing out "*:*" is a
// decision only the consumer can make, and it must cost an explicit, greppable line.
const (
	Wildcard       Resource = "*"
	WildcardAction Action   = "*"
)

// ResourceOf is the resource that protects a module: its own identity.
//
// This is the whole point of putting the vocabulary next to ModuleNaming. The convention
// "a module's ID is its RBAC resource" used to be a line in a document that nothing
// enforced; here it is a function, so the two can no longer disagree.
func ResourceOf(m ModuleNaming) Resource { return Resource(m.ModelName()) }

// Grant is one line of a policy: what a role may do to a resource.
// The zero value grants nothing — closed by default, like everything here.
type Grant struct {
	Resource Resource
	Actions  Action
}

// Matches reports whether a Grant covers a (resource, action) pair.
//
// This is the ONE place the wildcard is interpreted, so every enforcer agrees on what "*"
// means. Two implementations quietly disagreeing about that is a security hole nobody
// would ever see.
//
// An Action is one verb: a role that may read and write holds two Grants. Packing several
// actions into one string would bring back the parsing this vocabulary exists to remove.
func (g Grant) Matches(r Resource, a Action) bool {
	if g.Resource == "" || g.Actions == "" {
		return false // an empty grant grants nothing
	}
	if g.Resource != Wildcard && g.Resource != r {
		return false
	}
	return g.Actions == WildcardAction || g.Actions == a
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
