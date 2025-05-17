package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Angeldadro/Qraxis/src/katalyze_impl"
	"github.com/Angeldadro/Qraxis/src/types"
)

// UserCreatedEvent es un evento que se emite cuando se crea un usuario
type UserCreatedEvent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// MessageName devuelve el nombre del mensaje para enrutamiento
func (e *UserCreatedEvent) MessageName() string {
	return "user.created"
}

// Type devuelve el tipo de mensaje
func (e *UserCreatedEvent) Type() types.MessageType {
	return types.EventMessageType
}

// Payload devuelve el contenido del mensaje
func (e *UserCreatedEvent) Payload() interface{} {
	return e
}

// Validate asegura que el evento es válido
func (e *UserCreatedEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("ID no puede estar vacío")
	}
	return nil
}

// UserEventHandler maneja eventos relacionados con usuarios
type UserEventHandler struct {
	// Almacenamiento de eventos procesados
	processedEvents map[string]*UserCreatedEvent
	mu              sync.RWMutex
}

// NewUserEventHandler crea un nuevo manejador de eventos de usuarios
func NewUserEventHandler() *UserEventHandler {
	return &UserEventHandler{
		processedEvents: make(map[string]*UserCreatedEvent),
	}
}

// Handle procesa el evento
func (h *UserEventHandler) Handle(event types.Event) error {
	userEvent, ok := event.(*UserCreatedEvent)
	if !ok {
		return fmt.Errorf("tipo de evento inválido: %T", event)
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Almacenar el evento procesado
	h.processedEvents[userEvent.ID] = userEvent

	// Mostrar información del evento procesado
	log.Printf("[UserEventHandler] Evento procesado - Usuario creado: %s, Email: %s, Creado: %s",
		userEvent.Name,
		userEvent.Email,
		userEvent.CreatedAt.Format(time.RFC3339))

	return nil
}

// CanHandle verifica si este handler puede procesar el evento dado
func (h *UserEventHandler) CanHandle(event types.Event) bool {
	_, ok := event.(*UserCreatedEvent)
	return ok
}

// NotificationEventHandler es un segundo manejador que envía notificaciones
type NotificationEventHandler struct{}

// NewNotificationEventHandler crea un nuevo manejador de notificaciones
func NewNotificationEventHandler() *NotificationEventHandler {
	return &NotificationEventHandler{}
}

// Handle procesa el evento enviando una notificación
func (h *NotificationEventHandler) Handle(event types.Event) error {
	userEvent, ok := event.(*UserCreatedEvent)
	if !ok {
		return fmt.Errorf("tipo de evento inválido: %T", event)
	}

	// Simular envío de notificación
	log.Printf("[NotificationHandler] Notificación enviada - Bienvenido %s! Tu cuenta ha sido creada con éxito.",
		userEvent.Name)

	return nil
}

// CanHandle verifica si este handler puede procesar el evento dado
func (h *NotificationEventHandler) CanHandle(event types.Event) bool {
	_, ok := event.(*UserCreatedEvent)
	return ok
}

func main() {
	// Configurar el bus de eventos
	config := katalyze_impl.KatalyzeEventBusConfig{
		BootstrapServers: "localhost:9092",
		ClientID:         "qraxis-event-example",
		MaxRetries:       3,
		RetryInterval:    5000,
	}

	// Crear el bus de eventos
	eventBus, err := katalyze_impl.NewEventBus(config)
	if err != nil {
		log.Fatalf("Error al crear el bus de eventos: %v", err)
	}
	defer eventBus.Close()

	// Pre-registrar el productor para el evento
	if err := eventBus.PreRegisterProducer("user.created"); err != nil {
		log.Fatalf("Error al pre-registrar el productor: %v", err)
	}

	// Crear e inicializar los handlers de eventos
	userEventHandler := NewUserEventHandler()
	notificationHandler := NewNotificationEventHandler()

	// Registrar los handlers en el bus de eventos
	log.Println("Registrando manejador de eventos de usuarios...")
	err = eventBus.Subscribe("user.created", "process-user-created", userEventHandler, func() types.Event {
		return &UserCreatedEvent{}
	})
	if err != nil {
		log.Fatalf("Error al suscribir el handler de usuarios: %v", err)
	}

	log.Println("Registrando manejador de notificaciones...")
	err = eventBus.Subscribe("user.created", "send-welcome-notification", notificationHandler, func() types.Event {
		return &UserCreatedEvent{}
	})
	if err != nil {
		log.Fatalf("Error al suscribir el handler de notificaciones: %v", err)
	}

	log.Println("Ambos manejadores registrados correctamente")

	// Configurar manejo de señales para terminar limpiamente
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Iniciar una goroutine para publicar eventos periódicamente
	go func() {
		// Esperar un poco para que los componentes se inicialicen
		time.Sleep(3 * time.Second)
		log.Println("Iniciando publicación de eventos...")
		log.Println("Los eventos serán procesados por ambos manejadores: UserEventHandler y NotificationEventHandler")

		// Enviar eventos cada 5 segundos
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		// Datos de ejemplo para usuarios
		users := []struct {
			id    string
			name  string
			email string
		}{
			{"101", "Juan Pérez", "juan@ejemplo.com"},
			{"102", "María García", "maria@ejemplo.com"},
			{"103", "Carlos López", "carlos@ejemplo.com"},
			{"104", "Ana Martínez", "ana@ejemplo.com"},
		}
		index := 0

		for {
			select {
			case <-sigs:
				return
			case <-ticker.C:
				// Crear un evento
				user := users[index%len(users)]
				index++

				event := &UserCreatedEvent{
					ID:        user.id,
					Name:      user.name,
					Email:     user.email,
					CreatedAt: time.Now(),
				}

				log.Printf("Publicando evento de usuario creado - ID: %s, Nombre: %s",
					user.id, user.name)

				// Publicar el evento
				if err := eventBus.Publish(event); err != nil {
					log.Printf("Error al publicar evento: %v", err)
				}
			}
		}
	}()

	log.Println("Aplicación iniciada. Presione Ctrl+C para salir...")

	// Esperar señal de terminación
	<-sigs
	log.Println("Señal de terminación recibida, cerrando aplicación...")
}
