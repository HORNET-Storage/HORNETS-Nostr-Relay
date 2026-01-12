package nostr

var KindHandlers map[string]KindHandler

type KindWriter func(messageType string, params ...interface{})
type KindReader func() ([]byte, error)

type KindHandler func(read KindReader, write KindWriter)

func init() {
	KindHandlers = map[string]KindHandler{}
}

func RegisterHandler(kind string, handler func(read KindReader, write KindWriter)) error {
	KindHandlers[kind] = handler

	return nil
}

func GetHandler(kind string) func(read KindReader, write KindWriter) {
	handler, ok := KindHandlers[kind]

	if !ok {
		return nil
	}

	return handler
}

func GetHandlers() map[string]KindHandler {
	return KindHandlers
}

func ClearHandlers() {
	KindHandlers = make(map[string]KindHandler)
}
