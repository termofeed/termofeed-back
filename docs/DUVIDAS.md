# Dúvidas e Lacunas do Contrato — Termofeed Back-end

## 1. Contagem de Participantes (participantCount e participants.changed)
- **Problema:** O contrato REST (Room.participantCount) define o número de participantes registrados na sala (aumentado em POST /rooms/{code}/join). Já o evento AsyncAPI participants.changed é disparado quando \"alguém entra ou sai da sala\". Como a API não possui endpoint de logout/leave por REST, \"sair da sala\" só ocorre no fechamento da conexão WebSocket.
- **Decisão Adotada:** 
  1. participantCount no registro da sala reflete a quantidade de participantes registrados via join (respeitando maxParticipants).
  2. O evento WebSocket participants.changed transmite a contagem ativa de conexões conectadas à sala. Adicionalmente, quando novos participantes entram via join, o número total registrado é atualizado.

## 2. Ordenação de Empate no Ranking
- **Problema:** A AsyncAPI e o OpenAPI não especificavam explicitamente a regra de desempate no ranking de emojis caso dois emojis tivessem a mesma contagem.
- **Decisão Adotada:** Conforme RF e especificação do prompt, em caso de empate na contagem de reações, a ordenação segue estritamente a ordem definida na paleta de emojis da sala (llowedEmojis).

## 3. Retenção de Salas Encerradas e Código HTTP 410 vs Remoção
- **Problema:** O contrato diz que salas encerradas respondem com seus dados por até 24 horas (Room com status nded). Após 24h, respondem 410 Gone (ROOM_GONE). A especificação também estabelece que o sistema deve apagar os dados da sala em até 1 hora após as 24h.
- **Decisão Adotada:** Marcamos a sala como elegível a ROOM_GONE exatamente 24h após ndedAt. O expurgo definitivo da memória ocorre no ciclo de limpeza (background cleaner) para salas com mais de 25h de encerramento.
