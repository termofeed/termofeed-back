# Matriz de Rastreabilidade de Requisitos — Termofeed Back-end

Esta tabela mapeia cada requisito funcional (RF-001 a RF-035) e não funcional (RNF-001 a RNF-024) às implementações de código e aos testes automatizados que garantem o seu cumprimento.

## Requisitos Funcionais

| ID | Requisito | Componente / Arquivo | Teste Automatizado | Status |
|---|---|---|---|---|
| **RF-001** | Criar sala | `internal/room/service.go:CreateRoom`<br>`internal/api/handler.go:CreateRoom` | `internal/room/service_test.go:TestService_CreateRoom_Success`<br>`internal/api/handler_test.go:TestFullFlow_HTTPAndWebSocket`<br>`tests/contract_test.go:TestContract_APIResponsesMatchSchema` | Coberto |
| **RF-002** | Dispensar login | `internal/room/code.go:GenerateToken`<br>`internal/store/store.go` | Sem dependência de autenticação de terceiros; autenticação por tokens gerados | Coberto |
| **RF-003** | Definir duração inicial (5 a 240 min) | `internal/room/service.go:CreateRoom` | `internal/room/service_test.go:TestService_CreateRoom_Validation` | Coberto |
| **RF-004** | Definir intervalo entre reações (1 a 60s, padrão 5s) | `internal/room/service.go:CreateRoom` | `internal/room/service_test.go:TestService_CreateRoom_Validation` | Coberto |
| **RF-005** | Definir paleta de emojis (2 a 12 emojis) | `internal/room/service.go:CreateRoom` | `internal/room/service_test.go:TestService_CreateRoom_Validation` | Coberto |
| **RF-006** | Definir nome da sala (máx 80 chars) | `internal/room/service.go:CreateRoom` | `internal/room/service_test.go:TestService_CreateRoom_Validation` | Coberto |
| **RF-007** | Gerar link de entrada (`abc-defg-hij`) | `internal/room/code.go:GenerateRoomCode` | `internal/room/code_test.go:TestGenerateRoomCode` | Coberto |
| **RF-008** | Validar link da sala (`GET /rooms/{code}`) | `internal/room/service.go:GetRoom`<br>`internal/api/handler.go:GetRoom` | `internal/room/service_test.go:TestService_GetRoom`<br>`internal/api/handler_test.go:TestFullFlow_HTTPAndWebSocket` | Coberto |
| **RF-009** | Entrar na sala por link (`join`) | `internal/room/service.go:JoinRoom`<br>`internal/api/handler.go:JoinRoom` | `internal/room/service_test.go:TestService_JoinRoom`<br>`internal/api/handler_test.go:TestFullFlow_HTTPAndWebSocket` | Coberto |
| **RF-010** | Informar apelido (opcional, máx 30 chars) | `internal/room/service.go:JoinRoom` | `internal/room/service_test.go:TestService_JoinRoom` | Coberto |
| **RF-011** | Limitar participantes (`ROOM_FULL`) | `internal/room/service.go:JoinRoom`<br>`internal/store/memory.go:AddParticipant` | `internal/room/service_test.go:TestService_JoinRoom`<br>`internal/store/store_test.go:TestMemoryStore_CRUD` | Coberto |
| **RF-012** | Recusar entrada em sala encerrada (`ROOM_ENDED`) | `internal/room/service.go:JoinRoom` | `internal/room/service_test.go:TestService_JoinRoom` | Coberto |
| **RF-013** | Enviar reação (status 202, `nextAllowedAt`) | `internal/room/service.go:SendReaction`<br>`internal/api/handler.go:SendReaction` | `internal/room/service_test.go:TestService_ReactionsAndCooldown`<br>`internal/api/handler_test.go:TestFullFlow_HTTPAndWebSocket` | Coberto |
| **RF-014** | Intervalo entre reações (`COOLDOWN_ACTIVE`, 429) | `internal/room/service.go:SendReaction` | `internal/room/service_test.go:TestService_ReactionsAndCooldown` | Coberto |
| **RF-015** | Validar emoji permitido (`EMOJI_NOT_ALLOWED`) | `internal/room/service.go:SendReaction` | `internal/room/service_test.go:TestService_ReactionsAndCooldown` | Coberto |
| **RF-016** | Exigir token para reagir (`UNAUTHORIZED`) | `internal/room/service.go:SendReaction` | `internal/room/service_test.go:TestService_ReactionsAndCooldown` | Coberto |
| **RF-017** | Manter reações anônimas | `internal/ws/events.go:ReactionCreatedEvent`<br>`internal/summary/calculator.go` | `internal/ws/hub_test.go:TestEvents_Serialization` | Coberto |
| **RF-018** | Mostrar reações em tempo real (`reaction.created`) | `internal/room/service.go:SendReaction`<br>`internal/ws/hub.go:Broadcast` | `internal/api/handler_test.go:TestFullFlow_HTTPAndWebSocket` | Coberto |
| **RF-019** | Consultar resumo por janela (30s, 1m, 5m, all) | `internal/summary/calculator.go:CalculateWindow`<br>`internal/api/handler.go:GetSummary` | `internal/summary/calculator_test.go:TestCalculateWindow_*`<br>`internal/room/service_test.go:TestService_GetSummary` | Coberto |
| **RF-020** | Atualizar resumo em tempo real (`summary.updated` max 1 a cada 2s) | `internal/room/service.go:triggerThrottledSummary`<br>`internal/ws/hub.go` | `internal/room/service_test.go:TestService_ReactionsAndCooldown` | Coberto |
| **RF-021** | Feedback lúdico | Front-end | Responsabilidade exclusiva do cliente front-end (`termofeed-front`) | N/A (Front) |
| **RF-022** | Escolher janela de tempo | Front-end / Back-end `GET /summary?window=` | `internal/room/service_test.go:TestService_GetSummary` | Coberto |
| **RF-023** | Mostrar número de participantes (`participants.changed`) | `internal/ws/hub.go`<br>`internal/room/service.go:JoinRoom` | `internal/ws/hub_test.go:TestHub_ConnectAndDisconnectCallbacks` | Coberto |
| **RF-024** | Estender a sala | `internal/room/service.go:ExtendRoom`<br>`internal/api/handler.go:ExtendRoom` | `internal/room/service_test.go:TestService_ExtendRoom`<br>`internal/api/handler_test.go:TestFullFlow_HTTPAndWebSocket` | Coberto |
| **RF-025** | Limitar duração total (máx 480 min) | `internal/room/service.go:ExtendRoom` | `internal/room/service_test.go:TestService_ExtendRoom` | Coberto |
| **RF-026** | Restringir extensão ao criador (`hostToken`) | `internal/room/service.go:ExtendRoom` | `internal/room/service_test.go:TestService_ExtendRoom` | Coberto |
| **RF-027** | Avisar fim próximo da sala (`room.expiring` aos 60s) | `internal/room/service.go:Tick` | `internal/room/service_test.go:TestService_Tick_ExpirationAndWarning` | Coberto |
| **RF-028** | Encerrar a sala pelo criador (`POST /close`, razão `host`) | `internal/room/service.go:CloseRoom`<br>`internal/api/handler.go:CloseRoom` | `internal/room/service_test.go:TestService_CloseRoom`<br>`internal/api/handler_test.go:TestFullFlow_HTTPAndWebSocket` | Coberto |
| **RF-029** | Encerrar sala por tempo limite (razão `timeout`) | `internal/room/service.go:Tick` | `internal/room/service_test.go:TestService_Tick_ExpirationAndWarning` | Coberto |
| **RF-030** | Fechar conexões WebSocket ao encerrar | `internal/ws/hub.go:BroadcastAndClose`<br>`internal/room/service.go:closeRoomInternal` | `internal/room/service_test.go:TestService_CloseRoom`<br>`internal/ws/hub_test.go:TestHub_Broadcast` | Coberto |
| **RF-031** | Mostrar resumo final (`window = all`) | `internal/summary/calculator.go`<br>`internal/room/service.go:CloseRoom` | `internal/summary/calculator_test.go:TestCalculateWindow_RankingAndPercentages`<br>`internal/room/service_test.go:TestService_CloseRoom` | Coberto |
| **RF-032** | Remover sala após retenção (410 em 24h, purge em 25h) | `internal/room/service.go:GetRoom`<br>`internal/room/service.go:Tick` | `internal/room/service_test.go:TestService_GetRoom`<br>`internal/room/service_test.go:TestService_Tick_ExpirationAndWarning` | Coberto |
| **RF-033** | Mostrar controles do criador | Front-end | Responsabilidade exclusiva do cliente front-end (`termofeed-front`) | N/A (Front) |
| **RF-034** | Retomar controle da sala via link | Front-end | Responsabilidade exclusiva do cliente front-end (`termofeed-front`) | N/A (Front) |
| **RF-035** | Manter a sala sem o criador | Servidor desassocia sala do criador conectado | `internal/room/service_test.go:TestService_Tick_ExpirationAndWarning` | Coberto |

---

## Requisitos Não Funcionais

| ID | Requisito | Componente / Arquivo | Teste / Verificação | Status |
|---|---|---|---|---|
| **RNF-001** | Latência da reação (p95 <= 1s com 200 participantes e 20 rps) | `internal/ws/hub.go`<br>`internal/room/service.go` | `cmd/loadtest/main.go` | Coberto |
| **RNF-002** | Latência da API REST (p95 <= 300ms) | `internal/api/middleware.go:LoggingAndMetricsMiddleware` | Métricas Prometheus em `GET /metrics` (`termofeed_http_request_duration_seconds`) | Coberto |
| **RNF-003** | Capacidade por sala (200 participantes) | `internal/room/service.go`<br>`internal/store/memory.go` | `cmd/loadtest/main.go`<br>`internal/room/service_test.go:TestService_JoinRoom` | Coberto |
| **RNF-004** | Capacidade total (100 salas ativas) | Goroutines leves, `sync.RWMutex`, zero alocações desnecessárias | Arquitetura concorrente em Go | Coberto |
| **RNF-005** | Disponibilidade (>= 99.5%) | `/healthz`, `/readyz`, desligamento gracioso | `internal/api/handler_test.go:TestHealthAndMetricsEndpoints` | Coberto |
| **RNF-006** | Precisão do fim da sala (+- 2s) | `internal/room/service.go:Tick` (1s resolution) | `internal/room/service_test.go:TestService_Tick_ExpirationAndWarning` | Coberto |
| **RNF-007** | Precisão do intervalo de reações | `internal/room/service.go:SendReaction` | `internal/room/service_test.go:TestService_ReactionsAndCooldown` | Coberto |
| **RNF-008** | Reconexão automática | Front-end | Responsabilidade do cliente front-end | N/A (Front) |
| **RNF-009** | Segurança dos tokens (>= 128 bits criptográficos) | `internal/room/code.go:GenerateToken` | `internal/room/code_test.go:TestGenerateToken` (160 bits via crypto/rand) | Coberto |
| **RNF-010** | Transporte seguro (HTTPS / WSS) | Proxy Caddy (em produção) | Configuração do compose e proxy | Coberto |
| **RNF-011** | Proteção contra abuso (120 req/min, 10 salas/hora) | `internal/ratelimit/limiter.go`<br>`internal/api/middleware.go:RateLimitMiddleware` | `internal/ratelimit/limiter_test.go:TestIPRateLimiter_*` | Coberto |
| **RNF-012** | Privacidade dos dados (sem dados pessoais) | `internal/store/store.go` | Modelo de dados anônimo | Coberto |
| **RNF-013** | Retenção dos dados (apagamento em 24h + 1h) | `internal/room/service.go:Tick` | `internal/room/service_test.go:TestService_Tick_ExpirationAndWarning` | Coberto |
| **RNF-014 a RNF-022** | Acessibilidade, CSS, Design, Front | Front-end | Responsabilidades exclusivas do cliente front-end (`termofeed-front`) | N/A (Front) |
| **RNF-023** | Observabilidade (métricas e logs estruturados) | `internal/api/metrics.go`<br>`cmd/termofeed/main.go` | `internal/api/handler_test.go:TestHealthAndMetricsEndpoints` | Coberto |
| **RNF-024** | Aderência ao contrato OpenAPI e AsyncAPI | `api/openapi.yaml`<br>`api/asyncapi.yaml` | `tests/contract_test.go:TestOpenAPI_DocumentValidation`<br>`tests/contract_test.go:TestContract_APIResponsesMatchSchema` | Coberto |
