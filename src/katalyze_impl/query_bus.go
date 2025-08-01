package katalyze_impl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

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

type KatalyzeQueryBusConfig struct {
	BootstrapServers  string
	ClientID          string
	ResponseTimeoutMs int
}

type KatalyzeQueryBus struct {
	client           *client.Client
	bootstrapServers string
	producers        utils.TypedSyncMap[string, kTypes.ResponseProducer]
	consumers        utils.TypedSyncMap[string, kTypes.ResponseConsumer]
	ctx              context.Context
	cancelFunc       context.CancelFunc
}

func NewQueryBus(config KatalyzeQueryBusConfig) (*KatalyzeQueryBus, error) {
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

	return &KatalyzeQueryBus{
		client:           client,
		bootstrapServers: config.BootstrapServers,
		producers:        *utils.NewTypedSyncMap[string, kTypes.ResponseProducer](),
		consumers:        *utils.NewTypedSyncMap[string, kTypes.ResponseConsumer](),
		ctx:              ctx,
		cancelFunc:       cancelFunc,
	}, nil
}

func (b *KatalyzeQueryBus) Dispatch(query types.Query, timeoutMs int) (types.QueryResult, error) {
	if err := b.registerProducerIfNotExists(query.MessageName()); err != nil {
		return nil, err
	}
	producer, ok := b.producers.Load(query.MessageName())
	if !ok {
		return nil, errors.New("producer no encontrado")
	}

	marshaledQuery, err := json.Marshal(query)
	if err != nil {
		return nil, fmt.Errorf("error de serialización: %w", err)
	}
	res, err := producer.Produce(query.MessageName(), []byte(""), marshaledQuery, timeoutMs)
	if err != nil {
		return nil, fmt.Errorf("error al producir consulta: %w", err)
	}
	var remarshaledQuery types.QueryResult
	if err := json.Unmarshal(res, &remarshaledQuery); err != nil {
		return nil, fmt.Errorf("error al deserializar consulta: %w", err)
	}

	return remarshaledQuery, nil
}

func (b *KatalyzeQueryBus) registerProducerIfNotExists(messageName string) error {
	if _, exists := b.producers.Load(messageName); exists {
		return nil
	}
	producer, err := producer_builder.NewResponseProducerBuilder(b.bootstrapServers, messageName).
		Build()
	if err != nil {
		return err
	}
	if err := b.client.RegisterResponseProducer(producer); err != nil {
		return err
	}
	b.producers.Store(messageName, producer)
	return nil
}

func (b *KatalyzeQueryBus) registerConsumerIfNotExists(messageName string) error {
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

func (b *KatalyzeQueryBus) RegisterHandler(messageName string, handler types.QueryHandler, factory types.QueryFactory) error {
	if err := b.registerConsumerIfNotExists(messageName); err != nil {
		return err
	}
	consumer, ok := b.consumers.Load(messageName)
	if !ok {
		return errors.New("consumer no encontrado")
	}
	var responseFunc kTypes.ResponseHandler = func(message kTypes.Message) (interface{}, error) {
		query := factory()
		if err := json.Unmarshal(message.Value(), query); err != nil {
			return nil, err
		}
		return handler.Handle(query)
	}
	consumer.Subscribe(responseFunc)
	return nil
}

func (b *KatalyzeQueryBus) WarmUp(queryNames []string) {
	log.Println("[Qraxis] Iniciando calentamiento del QueryBus...")
	start := time.Now()
	var wg sync.WaitGroup

	for _, name := range queryNames {
		wg.Add(1)
		go func(queryName string) {
			defer wg.Done()
			log.Printf("[Qraxis WarmUp] Forzando inicialización del productor para la consulta '%s'...", queryName)
			if err := b.registerProducerIfNotExists(queryName); err != nil {
				log.Printf("[Qraxis WarmUp] Error al calentar el productor para '%s': %v", queryName, err)
			}
		}(name)
	}

	wg.Wait()
	log.Printf("[Qraxis] Calentamiento del QueryBus completado en %v.", time.Since(start))
}

func (b *KatalyzeQueryBus) Close() error {
	b.cancelFunc()
	b.client.Close()
	return nil
}
