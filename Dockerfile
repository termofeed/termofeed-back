# Multi-stage Dockerfile para Termofeed Back-end
FROM golang:1.27-alpine AS builder

WORKDIR /src

# Copiar definicoes de modulos primeiro para cache de dependencias
COPY go.mod go.sum ./
RUN go mod download

# Copiar codigo fonte
COPY . .

# Compilar binario estatico otimizado
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags "-s -w" \
    -o /bin/termofeed \
    ./cmd/termofeed

# Imagem final minima (Distroless non-root)
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /bin/termofeed /bin/termofeed

# Usuario sem privilegios (nonroot = uid 65532)
USER nonroot:nonroot

EXPOSE 8080

# Verificacao de saude usando o proprio binario compilado (sem depender de curl)
HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
    CMD ["/bin/termofeed", "healthcheck"]

ENTRYPOINT ["/bin/termofeed"]
