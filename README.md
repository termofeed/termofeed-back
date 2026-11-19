# Termofeed Back-end (Go)

> O humor da sala, em tempo real.

O **Termofeed** é um termômetro de humor interativo para reuniões e salas virtuais. Os participantes enviam reações de emojis que expressam seu sentimento, e o sistema agrega e transmite o ranking e o emoji dominante em tempo real por WebSocket.

Este repositório contém o **back-end em Go**, implementado conforme os contratos OpenAPI 3.1 e AsyncAPI 3.0.

---

## Recursos e Arquitetura

- **Go 1.22+**: HTTP server com biblioteca padrão e multiplexer `net/http` moderno.
- **WebSocket**: Biblioteca `github.com/coder/websocket` de alta performance.
- **Armazenamento em Memória**: Interface `Store` protegida por `sync.RWMutex`, com hash SHA-256 de tokens e comparação em tempo constante.
- **Relógio Injetável (`Clock`)**: Testes determinísticos para expiração, encerramento e retenção sem uso de `time.Sleep`.
- **Geração de Tipos**: Tipos gerados a partir do `openapi.yaml` via `oapi-codegen`.
- **Métricas e Logs**: Logs estruturados em JSON (`log/slog`) e métricas Prometheus em `GET /metrics`.
- **Containerização**: Imagem multi-stage mínima (< 30 MB) baseada em Distroless non-root, com healthcheck integrado no próprio binário.

---

## Como Rodar

### Sem Docker (desenvolvimento local)

Certifique-se de ter o Go 1.22 ou superior instalado:

```bash
# Copiar arquivo de configuracao inicial
cp .env.example .env

# Executar os testes
make test

# Iniciar o servidor
make run
```

O servidor iniciará escutando na porta `8080`.

### Com Docker Compose

```bash
# Subir o servico em segundo plano
docker compose up -d --build

# Ver logs
docker compose logs -f

# Parar o servico
docker compose down
```

---

## Variáveis de Ambiente

As configurações são definidas por variáveis de ambiente ou pelo arquivo `.env`:

| Variável | Padrão | Descrição |
|---|---|---|
| `PORT` | `8080` | Porta em que o servidor HTTP irá escutar. |
| `PUBLIC_BASE_URL` | `http://localhost:5173` | URL pública do front-end utilizada para compor o `joinUrl` das salas. |
| `ALLOWED_ORIGINS` | `http://localhost:5173` | Origens permitidas para CORS e conexões WebSocket (separadas por vírgula). |
| `MAX_PARTICIPANTS` | `200` | Limite padrão de participantes registrados por sala. |
| `RETENTION_HOURS` | `24` | Período em horas após o encerramento da sala em que os dados respondem `410 Gone`. |
| `LOG_LEVEL` | `info` | Nível do logger estruturado (`debug`, `info`, `warn`, `error`). |
| `TRUSTED_PROXY` | `false` | Se `true`, obtém o IP real do cliente via cabeçalho `X-Forwarded-For`. |

---

## Endpoints de Saúde e Métricas

| Endpoint | Método | Descrição |
|---|---|---|
| `/healthz` | `GET` | Verificação de vivacidade (Liveness probe). Responde HTTP 200 `{"status":"ok"}`. |
| `/readyz` | `GET` | Verificação de prontidão (Readiness probe). Responde HTTP 200 `{"status":"ok"}`. |
| `/metrics` | `GET` | Exportação de métricas em formato Prometheus. |

### Healthcheck por Binário

Para ambientes sem `curl` (como imagens mínimas baseadas em `scratch` ou `distroless`), utilize o próprio binário:

```bash
termofeed healthcheck
```
Retorna código de saída `0` se `/healthz` responder HTTP 200, ou `1` em caso de erro.

---

## Exemplos de Uso com cURL

### 1. Criar Sala
```bash
curl -X POST http://localhost:8080/api/v1/rooms \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Sprint Retrospective",
    "durationMinutes": 60,
    "reactionCooldownSeconds": 5
  }'
```
*Resposta (HTTP 201):* Devolve `room`, `hostToken` e `joinUrl`. **Guarde o `hostToken`.**

### 2. Consultar Sala Pública
```bash
curl http://localhost:8080/api/v1/rooms/abc-defg-hij
```

### 3. Entrar na Sala (`join`)
```bash
curl -X POST http://localhost:8080/api/v1/rooms/abc-defg-hij/join \
  -H "Content-Type: application/json" \
  -d '{
    "nickname": "Alice"
  }'
```
*Resposta (HTTP 201):* Devolve `participantId`, `participantToken` e `room`.

### 4. Enviar Reação
```bash
curl -X POST http://localhost:8080/api/v1/rooms/abc-defg-hij/reactions \
  -H "Content-Type: application/json" \
  -H "X-Participant-Token: <SEU_PARTICIPANT_TOKEN>" \
  -d '{
    "emoji": "😂"
  }'
```
*Resposta (HTTP 202):* Devolve `nextAllowedAt`.

### 5. Consultar Resumo por Janela
```bash
# Janelas suportadas: 30s, 1m, 5m, all
curl "http://localhost:8080/api/v1/rooms/abc-defg-hij/summary?window=1m"
```

### 6. Estender Sala (Criador)
```bash
curl -X POST http://localhost:8080/api/v1/rooms/abc-defg-hij/extend \
  -H "Content-Type: application/json" \
  -H "X-Host-Token: <SEU_HOST_TOKEN>" \
  -d '{
    "extraMinutes": 15
  }'
```

### 7. Encerrar Sala (Criador)
```bash
curl -X POST http://localhost:8080/api/v1/rooms/abc-defg-hij/close \
  -H "X-Host-Token: <SEU_HOST_TOKEN>"
```

### 8. Conectar ao WebSocket
```
ws://localhost:8080/api/v1/rooms/abc-defg-hij/ws?token=<SEU_PARTICIPANT_TOKEN>
```

---

## Comandos do Makefile

- `make run`: Executa a aplicação localmente.
- `make test`: Executa os testes unitários, de integração e de contrato com cobertura.
- `make lint`: Executa a verificação estática com `go vet`.
- `make gen`: Regenera os tipos da API a partir do `api/openapi.yaml`.
- `make build`: Compila o binário otimizado para produção em `bin/termofeed`.
- `make docker`: Constrói a imagem Docker localmente.
- `make loadtest`: Executa o teste de carga simulado com 200 participantes e 20 RPS.
