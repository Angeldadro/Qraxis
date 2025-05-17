package types

type Query interface {
	Message
}

type QueryResult interface{}

type QueryHandler interface {
	Handle(query Query) (QueryResult, error)
	CanHandle(query Query) bool
}

type QueryFactory func() Query

type QueryBus interface {
	Dispatch(query Query, timeoutMs int) (QueryResult, error)
	RegisterHandler(messageName string, handler QueryHandler, factory QueryFactory) error
	Close() error
}
