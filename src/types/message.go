package types

type MessageType string

const (
	CommandMessageType MessageType = "command"
	QueryMessageType   MessageType = "query"
	EventMessageType   MessageType = "event"
)

type Message interface {
	MessageName() string
	Type() MessageType
	Payload() interface{}
	Validate() error
}
