# First stage: Generate go.mod and go.sum
FROM golang:1.21.1-alpine AS builder

WORKDIR /app

# Copy your Go file.
COPY main.go .

# Initialize the module and download dependencies.
RUN go mod init go-api && \
    go mod tidy && \
    go mod download

# Build the application with CGO disabled to produce a statically linked binary.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Second stage: Use scratch
FROM scratch

# Copy the compiled Go binary from the first stage.
COPY --from=builder /app/main .

# Specify the port your app listens on.
EXPOSE 3000

# Set the entry point to your application.
ENTRYPOINT [ "./main" ]