# internal/

Each service has its own subdirectory with the following layers:

```
internal/<service>/
├── config/   # Viper + struct
├── server/   # HTTP (Gin) / gRPC / WS entry points and routing
├── service/  # Business use cases
├── repo/     # Data access (MySQL/Redis/Kafka)
└── model/    # Domain models + DTOs
```

The next iteration starts with `user-service` (login + JWT + refresh rotation),
then `room-service` (room list + follows + likes), followed by verification against the frontend MSW behavior.
