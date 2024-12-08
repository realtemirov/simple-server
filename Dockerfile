# Build stage
FROM golang:1.23-alpine AS build
WORKDIR /app

# Copy project files
COPY . .

# Build the application
RUN go build -o simple-server main.go

# Run stage from a smaller image
FROM alpine:latest
WORKDIR /app

# Copy the built binary from the build stage
COPY --from=build /app/simple-server .

# Expose port 8080
EXPOSE 8080

# Run the binary when the container starts
CMD ["./simple-server"]