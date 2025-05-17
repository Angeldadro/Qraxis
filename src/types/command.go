package types

type Command interface {
	Message
}

type CommandHandler interface {
	Handle(command Command) error
	CanHandle(command Command) bool
}
type CommandResult interface {
	Message
}

type CommandFactory func() Command
type CommandBus interface {
	Dispatch(command Command) error
	RegisterHandler(messageName string, handler CommandHandler, factory CommandFactory) (CommandResult, error)
	Close() error
}
