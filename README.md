# Qraxis

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-%3E%3D%201.24-blue.svg)

**Qraxis** es un framework de mensajería empresarial para Go que implementa patrones CQRS (Command Query Responsibility Segregation) y Event-Driven Architecture. Construido sobre [Katalyze](https://github.com/Angeldadro/Katalyze), proporciona una arquitectura robusta para sistemas distribuidos con buses de comandos, consultas y eventos.

## 🚀 Características principales

- ✅ **Arquitectura CQRS completa**: Separación clara de comandos, consultas y eventos
- ✅ **Implementación Event-Driven**: Sistema de eventos robusto y escalable
- ✅ **Buses de mensajería**: CommandBus, QueryBus y EventBus con interfaces limpias
- ✅ **Interfaces claras**: API intuitiva y fácil de usar
- ✅ **Alta cohesión, bajo acoplamiento**: Diseño modular para sistemas escalables

## 📦 Instalación

```bash
go get github.com/Angeldadro/Qraxis
```

## 🧩 Componentes principales

### Command Bus

El Command Bus permite enviar comandos (acciones que modifican el estado del sistema) a sus manejadores correspondientes:

- **Dispatch**: Envía comandos a sus manejadores
- **RegisterHandler**: Registra manejadores para tipos específicos de comandos
- **PreRegisterProducer**: Pre-registra productores para mejorar el rendimiento

### Query Bus

El Query Bus facilita la comunicación de consultas (solicitudes de información que no modifican el estado):

- **Dispatch**: Envía consultas y espera resultados
- **RegisterHandler**: Registra manejadores para tipos específicos de consultas
- **PreRegisterProducer**: Pre-registra productores para mejorar el rendimiento

### Event Bus

El Event Bus implementa el patrón publicador/suscriptor para eventos del sistema:

- **Publish**: Publica eventos en el bus
- **Subscribe**: Suscribe manejadores a tipos específicos de eventos
- **PreRegisterProducer/Consumer**: Pre-registra productores y consumidores

## 🔰 Ejemplos de uso

### Implementación de Command Bus

```go
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
commandBus.PreRegisterProducer("user.command")

// Registrar un manejador de comandos
commandBus.RegisterHandler("user.command", userCommandHandler, func() types.Command {
    return &UserCommand{}
})

// Enviar un comando
command := &UserCommand{ID: "1", Action: "create"}
if err := commandBus.Dispatch(command); err != nil {
    log.Printf("Error al despachar comando: %v", err)
}
```

### Implementación de Query Bus

```go
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
queryBus.PreRegisterProducer("user.query")

// Registrar un manejador de consultas
queryBus.RegisterHandler("user.query", userQueryHandler, func() types.Query {
    return &UserQuery{}
})

// Enviar una consulta y obtener resultado
query := &UserQuery{ID: "1"}
result, err := queryBus.Dispatch(query, 5000)
if err != nil {
    log.Printf("Error al despachar consulta: %v", err)
}
```

### Implementación de Event Bus

```go
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
eventBus.PreRegisterProducer("user.event")

// Registrar un manejador de eventos
eventBus.Subscribe("user.event", "process-user-event", userEventHandler, func() types.Event {
    return &UserEvent{}
})

// Publicar un evento
event := &UserEvent{
    ID:        "1",
    Name:      "Juan Pérez",
    Email:     "juan@ejemplo.com",
    CreatedAt: time.Now(),
}
if err := eventBus.Publish(event); err != nil {
    log.Printf("Error al publicar evento: %v", err)
}
```

## 🤝 Contribuir

Las contribuciones son bienvenidas. Puedes:

1. Reportar bugs o solicitar características a través de issues
2. Enviar Pull Requests con mejoras
3. Mejorar la documentación
4. Compartir ejemplos de uso

## 📜 Licencia

Este proyecto está licenciado bajo la Licencia MIT - ver el archivo LICENSE para más detalles.

---

Desarrollado con ❤️ por [@Angeldadro](https://github.com/Angeldadro)
