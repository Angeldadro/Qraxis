package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Angeldadro/Qraxis/src/katalyze_impl"
	"github.com/Angeldadro/Qraxis/src/types"
)

// CreateUserCommand es un comando para crear un usuario
type CreateUserCommand struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// MessageName devuelve el nombre del mensaje para enrutamiento
func (c *CreateUserCommand) MessageName() string {
	return "user.create"
}

// Type devuelve el tipo de mensaje
func (c *CreateUserCommand) Type() types.MessageType {
	return types.CommandMessageType
}

// Payload devuelve el contenido del mensaje
func (c *CreateUserCommand) Payload() interface{} {
	return c
}

// Validate asegura que el comando es válido
func (c *CreateUserCommand) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("ID no puede estar vacío")
	}
	if c.Name == "" {
		return fmt.Errorf("Name no puede estar vacío")
	}
	if c.Email == "" {
		return fmt.Errorf("Email no puede estar vacío")
	}
	return nil
}

// UserCommandHandler maneja comandos relacionados con usuarios
type UserCommandHandler struct {
	// Simulación de almacenamiento de usuarios
	users map[string]*CreateUserCommand
}

// NewUserCommandHandler crea un nuevo manejador de comandos de usuarios
func NewUserCommandHandler() *UserCommandHandler {
	return &UserCommandHandler{
		users: make(map[string]*CreateUserCommand),
	}
}

// Handle procesa el comando
func (h *UserCommandHandler) Handle(command types.Command) error {
	createUserCmd, ok := command.(*CreateUserCommand)
	if !ok {
		return fmt.Errorf("tipo de comando inválido: %T", command)
	}

	// Simular procesamiento
	log.Printf("Procesando comando para crear usuario: %s", createUserCmd.Name)
	time.Sleep(100 * time.Millisecond)

	// Almacenar el usuario
	h.users[createUserCmd.ID] = createUserCmd
	log.Printf("Usuario creado con ID: %s, Nombre: %s, Email: %s", 
		createUserCmd.ID, createUserCmd.Name, createUserCmd.Email)

	return nil
}

// CanHandle verifica si este handler puede procesar el comando dado
func (h *UserCommandHandler) CanHandle(command types.Command) bool {
	_, ok := command.(*CreateUserCommand)
	return ok
}

func main() {
	// Configurar el bus de comandos
	config := katalyze_impl.KatalyzeCommandBusConfig{
		BootstrapServers: "localhost:9092",
		ClientID:         "qraxis-command-example",
	}

	// Crear el bus de comandos
	commandBus, err := katalyze_impl.NewCommandBus(config)
	if err != nil {
		log.Fatalf("Error al crear el bus de comandos: %v", err)
	}
	defer commandBus.Close()

	// Pre-registrar el productor para el comando
	if err := commandBus.PreRegisterProducer("user.create"); err != nil {
		log.Fatalf("Error al pre-registrar el productor: %v", err)
	}

	// Crear e inicializar el handler
	userCommandHandler := NewUserCommandHandler()

	// Registrar el handler en el bus
	err = commandBus.RegisterHandler("user.create", userCommandHandler, func() types.Command {
		return &CreateUserCommand{}
	})
	if err != nil {
		log.Fatalf("Error al registrar el handler: %v", err)
	}

	// Configurar manejo de señales para terminar limpiamente
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Iniciar una goroutine para enviar comandos periódicamente
	go func() {
		// Esperar un poco para que los componentes se inicialicen
		time.Sleep(3 * time.Second)
		log.Println("Iniciando envío de comandos...")

		// Enviar comandos cada 5 segundos
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		// Datos de ejemplo para crear usuarios
		users := []struct {
			id    string
			name  string
			email string
		}{
			{"1", "Juan Pérez", "juan@ejemplo.com"},
			{"2", "María García", "maria@ejemplo.com"},
			{"3", "Carlos López", "carlos@ejemplo.com"},
		}
		index := 0

		for {
			select {
			case <-sigs:
				return
			case <-ticker.C:
				// Crear un comando
				user := users[index%len(users)]
				index++

				command := &CreateUserCommand{
					ID:    user.id,
					Name:  user.name,
					Email: user.email,
				}

				log.Printf("Enviando comando para crear usuario: %s", user.name)

				// Despachar el comando
				if err := commandBus.Dispatch(command); err != nil {
					log.Printf("Error al despachar comando: %v", err)
				}
			}
		}
	}()

	log.Println("Aplicación iniciada. Presione Ctrl+C para salir...")
	
	// Esperar señal de terminación
	<-sigs
	log.Println("Señal de terminación recibida, cerrando aplicación...")
}
