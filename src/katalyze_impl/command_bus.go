// Archivo: Qraxis/src/katalyze_impl/command_bus.go
package katalyze_impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	admin_builder "github.com/Angeldadro/Katalyze/src/builders/admin"
	client_builder "github.com/Angeldadro/Katalyze/src/builders/client"
	consumer_builder "github.com/Angeldadro/Katalyze/src/builders/consumer"
	producer_builder "github.com/Angeldadro/Katalyze/src/builders/producer"
	"github.com/Angeldadro/Katalyze/src/client"
	producer_helper "github.com/Angeldadro/Katalyze/src/helpers/producer"
	kTypes "github.com/Angeldadro/Katalyze/src/types"
	"github.com/Angeldadro/Qraxis/src/types"
	"github.com/Angeldadro/Qraxis/src/utils"
)

type GenericCommand struct {
	messageName string
	data        map[string]interface{}
}

func (c *GenericCommand) MessageName() string          { return c.messageName }
func (c *GenericCommand) Type() types.MessageType      { return types.CommandMessageType }
func (c *GenericCommand) Payload() interface{}         { return c.data }
func (c *GenericCommand) Validate() error              { return nil }
func NewGenericCommand(messageName string, data map[string]interface{}) *GenericCommand {
	return &GenericCommand{messageName: messageName, data: data}
}

type KatalyzeCommandBusConfig struct {
	BootstrapServers  string
	ClientID          string
	ResponseTimeoutMs int
	ProducerConfig    map[string]interface{} // <-- AÑADIDO
}

type KatalyzeCommandBus struct {
	client           *client.Client
	bootstrapServers string
	producers        utils.TypedSyncMap[string, kTypes.ResponseProducer]
	consumers        utils.TypedSyncMap[string, kTypes.ResponseConsumer]
	ctx              context.Context
	cancelFunc       context.CancelFunc
	producerConfig   map[string]interface{} // <-- AÑADIDO
}

func NewCommandBus(config KatalyzeCommandBusConfig) (*KatalyzeCommandBus, error) {
	if config.BootstrapServers == "" {
		return nil, errors.New("se requieren servidores bootstrap")
	}
	if config.ClientID == "" {
		return nil, errors.New("se requiere un ID de cliente")
	}
	if config.ResponseTimeoutMs <= 0 {
		config.ResponseTimeoutMs = 5000
	}
	adminClient, err := admin_builder.NewKafkaAdminClientBuilder(config.BootstrapServers).
		SetClientId(config.ClientID).
		Build()
	if err != nil {
		return nil, fmt.Errorf("error al crear cliente admin Katalyze: %w", err)
	}
	client, err := client_builder.NewClientBuilder().
		SetClientId(config.ClientID).
		SetAdminClient(adminClient).
		Build()
	if err != nil {
		return nil, fmt.Errorf("error al crear cliente Katalyze: %w", err)
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	return &KatalyzeCommandBus{
		client:           client,
		bootstrapServers: config.BootstrapServers,
		producers:        *utils.NewTypedSyncMap[string, kTypes.ResponseProducer](),
		consumers:        *utils.NewTypedSyncMap[string, kTypes.ResponseConsumer](),
		ctx:              ctx,
		cancelFunc:       cancelFunc,
		producerConfig:   config.ProducerConfig, // <-- AÑADIDO
	}, nil
}

func (b *KatalyzeCommandBus) Dispatch(command types.Command) error {
	if err := b.registerProducerIfNotExists(command.MessageName()); err != nil {
		return err
	}
	producer, ok := b.producers.Load(command.MessageName())
	if !ok {
		return errors.New("producer no encontrado")
	}
	marshaledCommand, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("error de serialización: %w", err)
	}
	timeoutMs := 5000
	res, err := producer.Produce(command.MessageName(), []byte(""), marshaledCommand, timeoutMs)
	if err != nil {
		return fmt.Errorf("error al producir comando: %w", err)
	}
	if res == nil {
		return errors.New("no se recibió respuesta del comando")
	}
	return nil
}

func (b *KatalyzeCommandBus) registerProducerIfNotExists(messageName string) error {
	if _, exists := b.producers.Load(messageName); exists {
		return nil
	}
	// --- INICIO DE MODIFICACIÓN ---
	builder := producer_builder.NewResponseProducerBuilder(b.bootstrapServers, messageName)
	if b.producerConfig != nil {
		for key, value := range b.producerConfig {
			builder.SetConfig(key, value)
		}
	}
	producer, err := builder.Build()
	// --- FIN DE MODIFICACIÓN ---
	if err != nil {
		return err
	}
	if err := b.client.RegisterResponseProducer(producer); err != nil {
		return err
	}
	b.producers.Store(messageName, producer)
	return nil
}

func (b *KatalyzeCommandBus) registerConsumerIfNotExists(messageName string) error {
	if _, exists := b.consumers.Load(messageName); exists {
		return nil
	}
	producer, err := producer_helper.CreateProducer(b.bootstrapServers, messageName)
	if err != nil {
		return err
	}
	consumer, err := consumer_builder.NewResponseConsumerBuilder(b.bootstrapServers, []string{messageName}).SetResponseProducer(producer).Build()
	if err != nil {
		return err
	}
	if err := b.client.RegisterConsumer(consumer); err != nil {
		return err
	}
	b.consumers.Store(messageName, consumer)
	return nil
}

func (b *KatalyzeCommandBus) RegisterHandler(messageName string, handler types.CommandHandler, factory types.CommandFactory) error {
	if err := b.registerConsumerIfNotExists(messageName); err != nil {
		return err
	}
	consumer, ok := b.consumers.Load(messageName)
	if !ok {
		return errors.New("consumer no encontrado")
	}
	var responseFunc kTypes.ResponseHandler = func(message kTypes.Message) (interface{}, error) {
		command := factory()
		if err := json.Unmarshal(message.Value(), command); err != nil {
			return nil, err
		}
		return handler.Handle(command)
	}
	consumer.Subscribe(responseFunc)
	return nil
}

func (b *KatalyzeCommandBus) Close() error {
	b.cancelFunc()
	b.client.Close()
	return nil
}