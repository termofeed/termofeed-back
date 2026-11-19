# Decisões de Arquitetura e Projeto (ADR) — Termofeed Back-end

## 1. Roteamento HTTP
- **Decisão:** Utilização do 
et/http padrão do Go (com suporte a método e wildcards como GET /api/v1/rooms/{code} introduzidos no Go 1.22+).
- **Motivação:** Manter o projeto com o menor número possível de dependências externas, excelente performance e sem overhead de frameworks externos.

## 2. WebSocket
- **Decisão:** Utilização da biblioteca github.com/coder/websocket (anteriormente 
hooyr.io/websocket).
- **Motivação:** Exigência da especificação da stack. Biblioteca moderna, idiomática para Go, baseada em contextos, com excelente gerenciamento de concorrência e buffer.

## 3. Armazenamento em Memória
- **Decisão:** Interface Store isolando o acesso aos dados, com implementação MemoryStore protegida por sync.RWMutex.
- **Motivação:** Conforme especificação para a versão 1.0. A abstração permite trocar no futuro por Redis ou PostgreSQL sem alterar a camada de domínio (internal/room).

## 4. Segurança e Hash de Tokens
- **Decisão:** Geração de hostToken e participantToken com 160 bits (20 bytes) via crypto/rand, codificados em base64 URL-safe (sem padding).
- **Armazenamento:** Os tokens são armazenados como hashes SHA-256 na store. As checagens utilizam subtle.ConstantTimeCompare para prevenir ataques de temporização (timing attacks).

## 5. Gerenciamento de Tempo e Agendamento
- **Decisão:** Abstração via interface Clock (RealClock e FakeClock).
- **Motivação:** Permitir testes determinísticos e instantâneos para expiração de salas, avisos de 60s (oom.expiring), encerramento por tempo limite e retenção de 24h, sem nunca utilizar 	ime.Sleep nos testes.
- **Ciclo de Vida da Sala:** Cada sala ativa é monitorada por um gerenciador de eventos temporizados atrelado ao Clock.

## 6. Agrupamento (Throttle) de Atualizações de Resumo
- **Decisão:** O evento summary.updated é limitado a no máximo 1 envio a cada 2 segundos por sala.
- **Implementação:** Quando ocorrem reações em sequência, uma atualização imediata ou agendada com debounce de 2s garante a taxa exigida na AsyncAPI.

## 7. Dependências Externas Utilizadas
- github.com/coder/websocket: biblioteca WebSocket exigida.
- github.com/prometheus/client_golang: exportação de métricas em GET /metrics.
- github.com/google/uuid: geração de UUID v4 para participantId.
- github.com/oapi-codegen/runtime: suporte aos tipos gerados a partir do OpenAPI 3.1.
- github.com/stretchr/testify: assertions e suites de teste.
- github.com/getkin/kin-openapi: validação de schema e conformidade de contrato nos testes.
