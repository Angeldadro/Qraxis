package types

type Command interface {
	Message
}
type CommandResult interface{}

type CommandHandler interface {
	Handle(command Command) (CommandResult, error)
	CanHandle(command Command) bool
}

type CommandFactory func() Command
type CommandBus interface {
	Dispatch(command Command) error
	RegisterHandler(messageName string, handler CommandHandler, factory CommandFactory) error
	// --- LÍNEA AÑADIDA ---
	WarmUp(commandNames []string)
	Close() error
}
