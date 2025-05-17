package katalyze_impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	admin_builder "github.com/Angeldadro/Katalyze/src/builders/admin"
	client_builder "github.com/Angeldadro/Katalyze/src/builders/client"
	consumer_builder "github.com/Angeldadro/Katalyze/src/builders/consumer"
	"github.com/Angeldadro/Katalyze/src/client"
	producer_helper "github.com/Angeldadro/Katalyze/src/helpers/producer"
	kTypes "github.com/Angeldadro/Katalyze/src/types"
	"github.com/Angeldadro/Qraxis/src/types"
	"github.com/Angeldadro/Qraxis/src/utils"
)

type KatalyzeEventBusConfig struct {
	BootstrapServers string
	ClientID         string
	MaxRetries       int
	RetryInterval    int
}

// KatalyzeEventBus implementa un único EventBus que maneja todos los eventos
type KatalyzeEventBus struct {
	client           *client.Client
	bootstrapServers string
	clientID         string
	producers        utils.TypedSyncMap[string, kTypes.SingleProducer]
	consumers        utils.TypedSyncMap[string, kTypes.RetryConsumer]
	maxRetries       int
	retryInterval    int
	ctx              context.Context
	cancelFunc       context.CancelFunc
}

// NewEventBus crea una nueva instancia del bus de eventos unificado
func NewEventBus(config KatalyzeEventBusConfig) (*KatalyzeEventBus, error) {
	if config.BootstrapServers == "" {
		return nil, errors.New("se requieren servidores bootstrap")
	}

	if config.ClientID == "" {
		return nil, errors.New("se requiere un ID de cliente")
	}

	if config.MaxRetries <= 0 {
		config.MaxRetries = 3
	}

	if config.RetryInterval <= 0 {
		config.RetryInterval = 5000
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

	return &KatalyzeEventBus{
		client:           client,
		bootstrapServers: config.BootstrapServers,
		clientID:         config.ClientID,
		producers:        *utils.NewTypedSyncMap[string, kTypes.SingleProducer](),
		consumers:        *utils.NewTypedSyncMap[string, kTypes.RetryConsumer](),
		maxRetries:       config.MaxRetries,
		retryInterval:    config.RetryInterval,
		ctx:              ctx,
		cancelFunc:       cancelFunc,
	}, nil
}

func (b *KatalyzeEventBus) Publish(event types.Event) error {
	if err := b.registerProducerIfNotExists(event.MessageName()); err != nil {
		return err
	}
	producer, ok := b.producers.Load(event.MessageName())
	if !ok {
		return errors.New("producer no encontrado")
	}

	marshaledEvent, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("error de serialización: %w", err)
	}

	return producer.Produce(event.MessageName(), "", marshaledEvent)
}

func (b *KatalyzeEventBus) registerProducerIfNotExists(messageName string) error {
	if _, exists := b.producers.Load(messageName); exists {
		return nil
	}
	producer, err := producer_helper.CreateProducer(b.bootstrapServers, messageName)
	if err != nil {
		return err
	}
	if err := b.client.RegisterProducer(producer); err != nil {
		return err
	}
	b.producers.Store(messageName, producer)
	return nil
}

func (b *KatalyzeEventBus) registerConsumerIfNotExists(messageName string, action string) error {
	if _, exists := b.consumers.Load(messageName); exists {
		return nil
	}

	producer, err := producer_helper.CreateRetryProducer(b.bootstrapServers, b.clientID)
	if err != nil {
		return err
	}

	consumerBuilder := consumer_builder.NewRetryConsumerBuilder(b.bootstrapServers, []string{messageName}, action, b.maxRetries)
	consumerBuilder.SetMaxRetries(b.maxRetries)
	consumerBuilder.SetProducer(producer)
	consumerBuilder.SetRetryInterval(b.retryInterval)
	consumer, err := consumerBuilder.Build()
	if err != nil {
		return err
	}
	if err := b.client.RegisterConsumer(consumer); err != nil {
		return err
	}
	b.consumers.Store(messageName, consumer)
	return nil
}

type EventFactory func() types.Event

func (b *KatalyzeEventBus) Subscribe(messageName string, action string, handler types.EventHandler, factory EventFactory) error {
	if err := b.registerConsumerIfNotExists(messageName, action); err != nil {
		return err
	}

	consumer, ok := b.consumers.Load(messageName)
	if !ok {
		return errors.New("consumer no encontrado")
	}

	consumer.Subscribe(func(msg kTypes.Message) error {
		event := factory()
		if err := json.Unmarshal(msg.Value(), event); err != nil {
			return err
		}
		return handler.Handle(event)
	})

	return nil
}

func (b *KatalyzeEventBus) PreRegisterProducer(messageName string) error {
	if _, exists := b.producers.Load(messageName); exists {
		return nil
	}
	return b.registerProducerIfNotExists(messageName)
}

func (b *KatalyzeEventBus) PreRegisterConsumer(messageName string, action string) error {
	if _, exists := b.consumers.Load(messageName); exists {
		return nil
	}
	return b.registerConsumerIfNotExists(messageName, action)
}

func (b *KatalyzeEventBus) Close() error {
	b.cancelFunc()
	b.client.Close()
	return nil
}
