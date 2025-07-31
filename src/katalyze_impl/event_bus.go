// Archivo: Qraxis/src/katalyze_impl/event_bus.go
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
	producer_types "github.com/Angeldadro/Katalyze/src/builders/producer/types"
	"github.com/Angeldadro/Katalyze/src/client"
	kTypes "github.com/Angeldadro/Katalyze/src/types"
	"github.com/Angeldadro/Qraxis/src/types"
	"github.com/Angeldadro/Qraxis/src/utils"
)

type KatalyzeEventBusConfig struct {
	BootstrapServers string
	ClientID         string
	MaxRetries       int
	RetryInterval    int
	ProducerConfig   map[string]interface{} // <-- AÑADIDO
}

type KatalyzeEventBus struct {
	client           *client.Client
	bootstrapServers string
	clientID         string
	producers        utils.TypedSyncMap[string, kTypes.SingleProducer]
	consumers        utils.TypedSyncMap[string, *utils.TypedSyncMap[string, kTypes.RetryConsumer]]
	maxRetries       int
	retryInterval    int
	ctx              context.Context
	cancelFunc       context.CancelFunc
	producerConfig   map[string]interface{} // <-- AÑADIDO
}

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
		consumers:        *utils.NewTypedSyncMap[string, *utils.TypedSyncMap[string, kTypes.RetryConsumer]](),
		maxRetries:       config.MaxRetries,
		retryInterval:    config.RetryInterval,
		ctx:              ctx,
		cancelFunc:       cancelFunc,
		producerConfig:   config.ProducerConfig, // <-- AÑADIDO
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

	// --- INICIO DE MODIFICACIÓN ---
	// Se reemplaza el helper por el builder para poder aplicar configuración personalizada.
	builder := producer_builder.NewSingleProducerBuilder(messageName, b.bootstrapServers)
	builder.SetClientId(b.clientID)

	if b.producerConfig != nil {
		// Aplicamos la configuración del preset de retry por defecto
		builder.SetAcks(producer_types.AcksAll)
		builder.SetCompressionType(producer_types.CompressionTypeSnappy)
		builder.SetMaxInFlightRequestsPerConnection(5)
		builder.SetLingerMs(10)

		// Sobrescribimos con la configuración del usuario
		for key, value := range b.producerConfig {
			switch key {
			case "enable.idempotence":
				if v, ok := value.(bool); ok {
					builder.SetEnableIdempotence(v)
				}
			}
		}
	}

	producer, err := builder.Build()
	// --- FIN DE MODIFICACIÓN ---

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
	actionMap, exists := b.consumers.Load(messageName)
	if !exists {
		actionMap = utils.NewTypedSyncMap[string, kTypes.RetryConsumer]()
		b.consumers.Store(messageName, actionMap)
	}
	if _, exists := actionMap.Load(action); exists {
		return nil
	}

	// --- INICIO DE MODIFICACIÓN ---
	// Crear un productor para reintentos usando el builder para aplicar configuración
	retryProducerName := fmt.Sprintf("%s-retry-producer", action)
	retryProducerBuilder := producer_builder.NewSingleProducerBuilder(retryProducerName, b.bootstrapServers)
	retryProducerBuilder.SetClientId(b.clientID)

	// Aplicamos configuración de reintentos por defecto
	retryProducerBuilder.SetAcks(producer_types.AcksAll)
	retryProducerBuilder.SetCompressionType(producer_types.CompressionTypeSnappy)
	retryProducerBuilder.SetEnableIdempotence(true)
	retryProducerBuilder.SetMaxInFlightRequestsPerConnection(5)
	retryProducerBuilder.SetLingerMs(10)

	// Sobrescribimos con la configuración del usuario
	if b.producerConfig != nil {
		for key, value := range b.producerConfig {
			switch key {
			case "enable.idempotence":
				if v, ok := value.(bool); ok {
					retryProducerBuilder.SetEnableIdempotence(v)
				}
			}
		}
	}
	producer, err := retryProducerBuilder.Build()
	// --- FIN DE MODIFICACIÓN ---
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
	actionMap.Store(action, consumer)
	return nil
}

func (b *KatalyzeEventBus) Subscribe(messageName string, action string, handler types.EventHandler, factory types.EventFactory) error {
	if err := b.registerConsumerIfNotExists(messageName, action); err != nil {
		return err
	}
	actionMap, ok := b.consumers.Load(messageName)
	if !ok {
		return errors.New("mapa de acciones no encontrado para el mensaje")
	}
	consumer, ok := actionMap.Load(action)
	if !ok {
		return errors.New("consumer no encontrado para la acción")
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

func (b *KatalyzeEventBus) Close() error {
	b.cancelFunc()
	b.client.Close()
	return nil
}
