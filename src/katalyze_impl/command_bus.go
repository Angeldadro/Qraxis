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

type KatalyzeCommandBusConfig struct {
	BootstrapServers  string
	ClientID          string
	ResponseTimeoutMs int
}

type KatalyzeCommandBus struct {
	client           *client.Client
	bootstrapServers string
	producers        utils.TypedSyncMap[string, kTypes.ResponseProducer]
	consumers        utils.TypedSyncMap[string, kTypes.ResponseConsumer]
	ctx              context.Context
	cancelFunc       context.CancelFunc
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

	timeoutMs := 10000 // Aumentamos el timeout por defecto
	_, err = producer.Produce(command.MessageName(), []byte(""), marshaledCommand, timeoutMs)
	if err != nil {
		return fmt.Errorf("error al producir comando: %w", err)
	}

	return nil
}

func (b *KatalyzeCommandBus) registerProducerIfNotExists(messageName string) error {
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

func (b *KatalyzeCommandBus) WarmUp(commandNames []string) {
	log.Println("[Qraxis] Iniciando calentamiento del CommandBus...")
	start := time.Now()
	var wg sync.WaitGroup

	for _, name := range commandNames {
		wg.Add(1)
		go func(cmdName string) {
			defer wg.Done()
			log.Printf("[Qraxis WarmUp] Forzando inicialización del productor para el comando '%s'...", cmdName)
			if err := b.registerProducerIfNotExists(cmdName); err != nil {
				log.Printf("[Qraxis WarmUp] Error al calentar el productor para '%s': %v", cmdName, err)
			}
		}(name)
	}

	wg.Wait()
	log.Printf("[Qraxis] Calentamiento del CommandBus completado en %v.", time.Since(start))
}

func (b *KatalyzeCommandBus) Close() error {
	b.cancelFunc()
	b.client.Close()
	return nil
}
