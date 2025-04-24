# Use an official Golang runtime as a parent image
FROM golang:1.21-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum to download dependencies
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code into the container
COPY . .

# Build the Go application
RUN go build -o stock-api main.go

# --- Final Stage: Create a minimal runtime image ---
FROM alpine:latest

# Set the working directory
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /app/stock-api .

# Copy any necessary environment files
COPY --from=builder /app/.env .

# Expose the port your Go application listens on (e.g., 8081)
EXPOSE 8081

# Command to run the application
CMD ["./stock-api"]

# Explicación del Dockerfile del Backend:

# FROM golang:1.21-alpine AS builder: Utiliza una imagen base de Go para construir la aplicación. La etapa se nombra builder.
# WORKDIR /app: Establece el directorio de trabajo dentro del contenedor.
# COPY go.mod go.sum ./: Copia los archivos de gestión de dependencias de Go.
# RUN go mod download: Descarga las dependencias de Go.
# COPY . .: Copia el código fuente de la aplicación.
# RUN go build -o stock-api main.go: Compila la aplicación Go y genera un ejecutable llamado stock-api.
# FROM alpine:latest: Utiliza una imagen base Alpine Linux más pequeña para la etapa final de la imagen.
# COPY --from=builder /app/stock-api .: Copia el ejecutable construido desde la etapa builder.
# COPY --from=builder /app/.env .: Copia el archivo .env si lo necesitas en el runtime.
# EXPOSE 8081: Expone el puerto en el que tu API Go escucha.
# CMD ["./stock-api"]: Define el comando para ejecutar la aplicación cuando el contenedor se inicie.