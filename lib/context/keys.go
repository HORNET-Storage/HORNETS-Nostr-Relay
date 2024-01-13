package context

type ContextKey struct {
	key string
}

var (
	Storage = &ContextKey{key: "Storage"}
	Host    = &ContextKey{key: "Host"}

	PrivateKey = &ContextKey{key: "PrivateKey"}
	PublicKey  = &ContextKey{key: "PublicKey"}
	Address    = &ContextKey{key: "Address"}
)
