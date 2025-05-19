package types

type Event interface {
	Message
}

type EventHandler interface {
	Handle(event Event) error
	CanHandle(event Event) bool
}
type EventFactory func() Event
type EventBus interface {
	Publish(event Event) error
	Subscribe(messageName string, action string, handler EventHandler, factory EventFactory) error
	Close() error
}
