package main

import (
	"encoding/json"
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

// UserQuery es una consulta para obtener información de un usuario
type UserQuery struct {
	ID string `json:"id"`
}

// MessageName devuelve el nombre del mensaje para enrutamiento
func (q *UserQuery) MessageName() string {
	return "user.query"
}

// Type devuelve el tipo de mensaje
func (q *UserQuery) Type() types.MessageType {
	return types.QueryMessageType
}

// Payload devuelve el contenido del mensaje
func (q *UserQuery) Payload() interface{} {
	return q
}

// Validate asegura que la consulta es válida
func (q *UserQuery) Validate() error {
	if q.ID == "" {
		return fmt.Errorf("ID no puede estar vacío")
	}
	return nil
}

// UserResponse es la respuesta a UserQuery
type UserResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

// UserQueryHandler maneja consultas de tipo UserQuery
type UserQueryHandler struct {
	// Simulación de una base de datos de usuarios
	users map[string]*UserResponse
	mu    sync.RWMutex
}

// NewUserQueryHandler crea un nuevo manejador de consultas de usuarios
func NewUserQueryHandler() *UserQueryHandler {
	// Crear algunos usuarios de ejemplo
	return &UserQueryHandler{
		users: map[string]*UserResponse{
			"1": {
				ID:        "1",
				Name:      "Juan Pérez",
				Email:     "juan@ejemplo.com",
				CreatedAt: time.Now().Format(time.RFC3339),
			},
			"2": {
				ID:        "2",
				Name:      "María García",
				Email:     "maria@ejemplo.com",
				CreatedAt: time.Now().Format(time.RFC3339),
			},
			"3": {
				ID:        "3",
				Name:      "Carlos López",
				Email:     "carlos@ejemplo.com",
				CreatedAt: time.Now().Format(time.RFC3339),
			},
		},
	}
}

// Handle procesa la consulta y devuelve el resultado
func (h *UserQueryHandler) Handle(query types.Query) (types.QueryResult, error) {
	userQuery, ok := query.(*UserQuery)
	if !ok {
		return nil, fmt.Errorf("tipo de consulta inválido: %T", query)
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	user, exists := h.users[userQuery.ID]
	if !exists {
		return nil, fmt.Errorf("usuario con ID %s no encontrado", userQuery.ID)
	}

	// Simular algún procesamiento
	log.Printf("Procesando consulta para usuario: %s", userQuery.ID)
	time.Sleep(100 * time.Millisecond)

	return user, nil
}

// CanHandle verifica si este handler puede procesar la consulta dada
func (h *UserQueryHandler) CanHandle(query types.Query) bool {
	_, ok := query.(*UserQuery)
	return ok
}

func main() {
	// Configurar el bus de consultas
	config := katalyze_impl.KatalyzeQueryBusConfig{
		BootstrapServers: "localhost:9092",
		ClientID:         "qraxis-query-example",
	}

	// Crear el bus de consultas
	queryBus, err := katalyze_impl.NewQueryBus(config)
	if err != nil {
		log.Fatalf("Error al crear el bus de consultas: %v", err)
	}
	defer queryBus.Close()

	// Pre-registrar el productor para la consulta
	if err := queryBus.PreRegisterProducer("user.query"); err != nil {
		log.Fatalf("Error al pre-registrar el productor: %v", err)
	}

	// Crear e inicializar el handler
	userQueryHandler := NewUserQueryHandler()

	// Registrar el handler en el bus
	err = queryBus.RegisterHandler("user.query", userQueryHandler, func() types.Query {
		return &UserQuery{}
	})
	if err != nil {
		log.Fatalf("Error al registrar el handler: %v", err)
	}

	// Configurar manejo de señales para terminar limpiamente
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Iniciar una goroutine para enviar consultas periódicamente
	go func() {
		// Esperar un poco para que los componentes se inicialicen
		time.Sleep(3 * time.Second)
		log.Println("Iniciando envío de consultas...")

		// Enviar consultas cada 5 segundos
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		// Alternar entre consultar usuarios
		userIDs := []string{"1", "2", "3"}
		index := 0

		for {
			select {
			case <-sigs:
				return
			case <-ticker.C:
				// Crear una consulta
				userID := userIDs[index%len(userIDs)]
				index++

				query := &UserQuery{ID: userID}

				log.Printf("Enviando consulta para usuario ID: %s", userID)

				// Despachar la consulta
				result, err := queryBus.Dispatch(query, 5000)
				if err != nil {
					log.Printf("Error al despachar consulta: %v", err)
					continue
				}

				// Remarshalizar para mostrar el resultado
				resultBytes, err := json.Marshal(result)
				if err != nil {
					log.Printf("Error al serializar resultado: %v", err)
					continue
				}
				var userResponse UserResponse
				if err := json.Unmarshal(resultBytes, &userResponse); err != nil {
					log.Printf("Error al deserializar resultado: %v", err)
					continue
				}

				// Mostrar el resultado
				log.Printf("Respuesta recibida para usuario %s: Nombre: %s, Email: %s, Creado: %s",
					userResponse.ID, userResponse.Name, userResponse.Email, userResponse.CreatedAt)
			}
		}
	}()

	log.Println("Aplicación iniciada. Presione Ctrl+C para salir...")
	
	// Esperar señal de terminación
	<-sigs
	log.Println("Señal de terminación recibida, cerrando aplicación...")
}
