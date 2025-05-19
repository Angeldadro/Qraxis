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
	RegisterHandler(messageName string, handler CommandHandler, factory CommandFactory) error

	// PreRegisterProducer pre-registra un productor para un tipo de comando específico
	PreRegisterProducer(messageName string) error

	// Close libera los recursos utilizados por el bus de comandos
	Close() error
}
