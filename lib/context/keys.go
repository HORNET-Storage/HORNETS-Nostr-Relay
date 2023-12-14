package context

type ContextKey struct {
	key string
}

var (
	BlockDatabase   = &ContextKey{key: "BlockDatabase"}
	ContentDatabase = &ContextKey{key: "ContentDatabase"}
	CacheDatabase   = &ContextKey{key: "CacheDatabase"}
	Storage         = &ContextKey{key: "Storage"}
	Host            = &ContextKey{key: "Host"}

	PrivateKey = &ContextKey{key: "PrivateKey"}
	PublicKey  = &ContextKey{key: "PublicKey"}
	Address    = &ContextKey{key: "Address"}
)
