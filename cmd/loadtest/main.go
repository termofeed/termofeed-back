package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/termofeed/termofeed-back/internal/api/gen"
)

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "Base URL of Termofeed server")
	participantsCount := flag.Int("participants", 200, "Number of participants")
	rps := flag.Int("rps", 20, "Reactions per second")
	durationSec := flag.Int("duration", 10, "Duration of the test in seconds")
	flag.Parse()

	fmt.Printf("=== Iniciando teste de carga Termofeed (RNF-001) ===\n")
	fmt.Printf("Servidor: %s\nParticipantes: %d\nRPS: %d\nDuração: %ds\n", *baseURL, *participantsCount, *rps, *durationSec)

	// 1. Create Room
	createReq := gen.CreateRoomRequest{
		DurationMinutes:         60,
		ReactionCooldownSeconds: intPtr(1), // set small cooldown to allow RPS across participants
		MaxParticipants:         participantsCount,
	}
	body, _ := json.Marshal(createReq)
	resp, err := http.Post(*baseURL+"/api/v1/rooms", "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("Falha ao criar sala: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	var createResp gen.CreateRoomResponse
	if err := json.NewDecoder(resp.Body).Decode(&createResp); err != nil {
		fmt.Printf("Falha ao decodificar resposta da sala: %v\n", err)
		os.Exit(1)
	}
	roomCode := createResp.Room.Code
	fmt.Printf("Sala criada com sucesso: %s\n", roomCode)

	// 2. Join 200 participants
	fmt.Printf("Registrando %d participantes...\n", *participantsCount)
	tokens := make([]string, *participantsCount)
	for i := 0; i < *participantsCount; i++ {
		nick := fmt.Sprintf("User-%d", i+1)
		jReq := gen.JoinRequest{Nickname: &nick}
		jBody, _ := json.Marshal(jReq)
		jResp, err := http.Post(*baseURL+"/api/v1/rooms/"+roomCode+"/join", "application/json", bytes.NewReader(jBody))
		if err != nil {
			fmt.Printf("Falha ao registrar participante %d: %v\n", i+1, err)
			os.Exit(1)
		}
		var joinRes gen.JoinResponse
		_ = json.NewDecoder(jResp.Body).Decode(&joinRes)
		jResp.Body.Close()
		tokens[i] = joinRes.ParticipantToken
	}

	// 3. Connect WebSockets for all participants
	fmt.Printf("Conectando WebSockets dos participantes...\n")
	wsURLBase := "ws" + strings.TrimPrefix(*baseURL, "http") + "/api/v1/rooms/" + roomCode + "/ws?token="
	conns := make([]*websocket.Conn, *participantsCount)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wgConnect sync.WaitGroup
	for i := 0; i < *participantsCount; i++ {
		wgConnect.Add(1)
		go func(idx int) {
			defer wgConnect.Done()
			c, _, err := websocket.Dial(ctx, wsURLBase+tokens[idx], nil)
			if err != nil {
				fmt.Printf("Erro conectando WS %d: %v\n", idx, err)
				return
			}
			conns[idx] = c
		}(i)
	}
	wgConnect.Wait()
	fmt.Printf("Todos os WebSockets conectados com sucesso.\n")

	// 4. Measure reaction delivery latency
	var latenciesLock sync.Mutex
	var latencies []time.Duration
	var totalReactionsSent int64
	var totalReactionsReceived int64

	// Start reader on participant 0 to measure delivery latency
	readerCtx, readerCancel := context.WithCancel(ctx)
	defer readerCancel()

	reactionTimestamps := sync.Map{}

	go func() {
		c := conns[0]
		if c == nil {
			return
		}
		for {
			select {
			case <-readerCtx.Done():
				return
			default:
				_, data, err := c.Read(readerCtx)
				if err != nil {
					return
				}
				var ev map[string]interface{}
				if err := json.Unmarshal(data, &ev); err == nil && ev["type"] == "reaction.created" {
					atomic.AddInt64(&totalReactionsReceived, 1)
					// Record latency
					emoji, _ := ev["emoji"].(string)
					if val, ok := reactionTimestamps.LoadAndDelete(emoji); ok {
						sentAt := val.(time.Time)
						latency := time.Since(sentAt)
						latenciesLock.Lock()
						latencies = append(latencies, latency)
						latenciesLock.Unlock()
					}
				}
			}
		}
	}()

	// 5. Send reactions at target RPS
	ticker := time.NewTicker(time.Second / time.Duration(*rps))
	defer ticker.Stop()
	testEnd := time.Now().Add(time.Duration(*durationSec) * time.Second)

	fmt.Printf("Iniciando envio de reações a %d RPS...\n", *rps)
	pIdx := 0
	for time.Now().Before(testEnd) {
		<-ticker.C
		token := tokens[pIdx%len(tokens)]
		pIdx++

		sentTime := time.Now()
		req, _ := http.NewRequest(http.MethodPost, *baseURL+"/api/v1/rooms/"+roomCode+"/reactions", strings.NewReader(`{"emoji":"😂"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Participant-Token", token)

		reactionTimestamps.Store("😂", sentTime)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusAccepted {
				atomic.AddInt64(&totalReactionsSent, 1)
			}
		}
	}

	time.Sleep(1 * time.Second)

	// Calculate latency p95
	latenciesLock.Lock()
	defer latenciesLock.Unlock()

	fmt.Printf("\n=== Resultados do Teste de Carga ===\n")
	fmt.Printf("Reações enviadas: %d\n", atomic.LoadInt64(&totalReactionsSent))
	fmt.Printf("Reações recebidas no WS: %d\n", atomic.LoadInt64(&totalReactionsReceived))

	if len(latencies) == 0 {
		fmt.Printf("Nenhuma latência medida.\n")
		return
	}

	sort.Slice(latencies, func(i, j int) bool {
		return latencies[i] < latencies[j]
	})

	p95Idx := int(float64(len(latencies)) * 0.95)
	p95 := latencies[p95Idx]
	median := latencies[len(latencies)/2]
	min := latencies[0]
	max := latencies[len(latencies)-1]

	fmt.Printf("Latência Mínima: %v\n", min)
	fmt.Printf("Latência Mediana (p50): %v\n", median)
	fmt.Printf("Latência p95: %v (RNF-001 limite: 1s)\n", p95)
	fmt.Printf("Latência Máxima: %v\n", max)

	if p95 <= time.Second {
		fmt.Printf("SUCESSO: Latência p95 está dentro do critério de aceite (<= 1s).\n")
	} else {
		fmt.Printf("FALHA: Latência p95 excedeu 1s.\n")
	}
}

func intPtr(i int) *int {
	return &i
}
