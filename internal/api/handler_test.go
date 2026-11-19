package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/termofeed/termofeed-back/internal/api"
	"github.com/termofeed/termofeed-back/internal/api/gen"
	"github.com/termofeed/termofeed-back/internal/clock"
	"github.com/termofeed/termofeed-back/internal/config"
	"github.com/termofeed/termofeed-back/internal/ratelimit"
	"github.com/termofeed/termofeed-back/internal/room"
	"github.com/termofeed/termofeed-back/internal/store"
	"github.com/termofeed/termofeed-back/internal/ws"
)

func setupTestServer(t *testing.T) (*httptest.Server, *room.Service, *clock.FakeClock) {
	fc := clock.NewFakeClock(time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC))
	memStore := store.NewMemoryStore()
	hub := ws.NewHub()
	cfg := &config.Config{
		Port:           "8080",
		PublicBaseURL:  "http://localhost:5173",
		AllowedOrigins: []string{"*"},
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	limiter := ratelimit.NewIPRateLimiter(fc)
	reg := prometheus.NewRegistry()

	svc := room.NewService(memStore, fc, hub, cfg.PublicBaseURL, 24)
	handler := api.NewHandler(svc, hub, cfg, logger, reg)
	router := api.BuildRouter(handler, limiter)

	server := httptest.NewServer(router)
	t.Cleanup(func() {
		server.Close()
	})

	return server, svc, fc
}

func TestHealthAndMetricsEndpoints(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	// Healthz
	resp, err := http.Get(ts.URL + "/healthz")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Readyz
	resp, err = http.Get(ts.URL + "/readyz")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Metrics
	resp, err = http.Get(ts.URL + "/metrics")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestFullFlow_HTTPAndWebSocket(t *testing.T) {
	ts, _, fc := setupTestServer(t)

	// 1. Create Room (POST /api/v1/rooms)
	createReq := gen.CreateRoomRequest{
		DurationMinutes: 30,
	}
	body, _ := json.Marshal(createReq)
	resp, err := http.Post(ts.URL+"/api/v1/rooms", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp gen.CreateRoomResponse
	err = json.NewDecoder(resp.Body).Decode(&createResp)
	require.NoError(t, err)
	roomCode := createResp.Room.Code
	hostToken := createResp.HostToken

	assert.NotEmpty(t, roomCode)
	assert.NotEmpty(t, hostToken)

	// 2. Get Room (GET /api/v1/rooms/{code})
	resp, err = http.Get(ts.URL + "/api/v1/rooms/" + roomCode)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var roomData gen.Room
	err = json.NewDecoder(resp.Body).Decode(&roomData)
	require.NoError(t, err)
	assert.Equal(t, roomCode, roomData.Code)

	// 3. Join Room (POST /api/v1/rooms/{code}/join)
	nick := "Alice"
	joinReq := gen.JoinRequest{Nickname: &nick}
	body, _ = json.Marshal(joinReq)
	resp, err = http.Post(ts.URL+"/api/v1/rooms/"+roomCode+"/join", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var joinResp gen.JoinResponse
	err = json.NewDecoder(resp.Body).Decode(&joinResp)
	require.NoError(t, err)
	participantToken := joinResp.ParticipantToken
	assert.NotEmpty(t, participantToken)

	// 4. Connect WebSocket (GET /api/v1/rooms/{code}/ws?token=...)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/v1/rooms/" + roomCode + "/ws?token=" + participantToken
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	wsConn, _, err := websocket.Dial(ctx, wsURL, nil)
	require.NoError(t, err)
	defer wsConn.Close(websocket.StatusNormalClosure, "")

	// 5. Send Reaction (POST /api/v1/rooms/{code}/reactions)
	reactReq := gen.ReactionRequest{Emoji: "😂"}
	body, _ = json.Marshal(reactReq)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomCode+"/reactions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Participant-Token", participantToken)

	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// 6. Verify reaction event received on WebSocket
	_, msg, err := wsConn.Read(ctx)
	require.NoError(t, err)
	var event map[string]interface{}
	err = json.Unmarshal(msg, &event)
	require.NoError(t, err)
	assert.Equal(t, "reaction.created", event["type"])
	assert.Equal(t, "😂", event["emoji"])

	// 7. Get Summary (GET /api/v1/rooms/{code}/summary?window=1m)
	resp, err = http.Get(ts.URL + "/api/v1/rooms/" + roomCode + "/summary?window=1m")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var summaryResp gen.Summary
	err = json.NewDecoder(resp.Body).Decode(&summaryResp)
	require.NoError(t, err)
	assert.Equal(t, 1, summaryResp.TotalReactions)
	assert.Equal(t, "😂", *summaryResp.DominantEmoji)

	// 8. Extend Room (POST /api/v1/rooms/{code}/extend)
	extendReq := gen.ExtendRequest{ExtraMinutes: 15}
	body, _ = json.Marshal(extendReq)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomCode+"/extend", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Host-Token", hostToken)

	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Verify room.extended on WebSocket (or summary.updated)
	// Read WS until we get room.extended
	readExtended := false
	for i := 0; i < 3; i++ {
		_, msg, err = wsConn.Read(ctx)
		if err != nil {
			break
		}
		var ev map[string]interface{}
		_ = json.Unmarshal(msg, &ev)
		if ev["type"] == "room.extended" {
			readExtended = true
			break
		}
	}
	assert.True(t, readExtended)

	// 9. Close Room (POST /api/v1/rooms/{code}/close)
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/v1/rooms/"+roomCode+"/close", nil)
	req.Header.Set("X-Host-Token", hostToken)

	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var closedResp gen.ClosedRoom
	err = json.NewDecoder(resp.Body).Decode(&closedResp)
	require.NoError(t, err)
	assert.Equal(t, gen.Ended, closedResp.Room.Status)
	assert.Equal(t, 1, closedResp.Summary.TotalReactions)

	_ = fc
}
