package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
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

func TestOpenAPI_DocumentValidation(t *testing.T) {
	ctx := context.Background()
	loader := openapi3.NewLoader()

	doc, err := loader.LoadFromFile("../api/openapi.yaml")
	if err != nil {
		// Fallback for execution from root directory
		doc, err = loader.LoadFromFile("api/openapi.yaml")
	}
	require.NoError(t, err, "openapi.yaml must load successfully")
	require.NotNil(t, doc)

	err = doc.Validate(ctx)
	require.NoError(t, err, "openapi.yaml must be valid OpenAPI 3.1")
}

func TestAsyncAPI_SpecificationExists(t *testing.T) {
	var path string
	if _, err := os.Stat("../api/asyncapi.yaml"); err == nil {
		path = "../api/asyncapi.yaml"
	} else if _, err := os.Stat("api/asyncapi.yaml"); err == nil {
		path = "api/asyncapi.yaml"
	}
	require.NotEmpty(t, path, "asyncapi.yaml must exist")

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "asyncapi: 3.0.0")
	assert.Contains(t, string(data), "reactionCreated")
	assert.Contains(t, string(data), "summaryUpdated")
	assert.Contains(t, string(data), "participantsChanged")
	assert.Contains(t, string(data), "roomExpiring")
	assert.Contains(t, string(data), "roomExtended")
	assert.Contains(t, string(data), "roomClosed")
}

func TestContract_APIResponsesMatchSchema(t *testing.T) {
	ctx := context.Background()
	loader := openapi3.NewLoader()

	docPath := "../api/openapi.yaml"
	if _, err := os.Stat(docPath); err != nil {
		docPath = "api/openapi.yaml"
	}
	doc, err := loader.LoadFromFile(docPath)
	require.NoError(t, err)

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
	defer server.Close()

	// 1. Validate CreateRoomResponse schema
	createSchema := doc.Components.Schemas["CreateRoomResponse"].Value
	require.NotNil(t, createSchema)

	reqBody, _ := json.Marshal(gen.CreateRoomRequest{DurationMinutes: 60})
	resp, err := http.Post(server.URL+"/api/v1/rooms", "application/json", bytes.NewReader(reqBody))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var createData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&createData)
	require.NoError(t, err)

	err = createSchema.VisitJSON(createData)
	assert.NoError(t, err, "CreateRoom response must match OpenAPI schema")

	roomMap := createData["room"].(map[string]interface{})
	roomCode := roomMap["code"].(string)
	hostToken := createData["hostToken"].(string)

	// 2. Validate Room schema from GET /rooms/{code}
	roomSchema := doc.Components.Schemas["Room"].Value
	require.NotNil(t, roomSchema)

	resp, err = http.Get(server.URL + "/api/v1/rooms/" + roomCode)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var getRoomData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&getRoomData)
	require.NoError(t, err)

	err = roomSchema.VisitJSON(getRoomData)
	assert.NoError(t, err, "GET /rooms/{code} response must match Room schema")

	// 3. Validate JoinResponse schema
	joinSchema := doc.Components.Schemas["JoinResponse"].Value
	require.NotNil(t, joinSchema)

	nick := "ContractTester"
	joinReqBody, _ := json.Marshal(gen.JoinRequest{Nickname: &nick})
	resp, err = http.Post(server.URL+"/api/v1/rooms/"+roomCode+"/join", "application/json", bytes.NewReader(joinReqBody))
	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var joinData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&joinData)
	require.NoError(t, err)

	err = joinSchema.VisitJSON(joinData)
	assert.NoError(t, err, "Join response must match JoinResponse schema")
	pToken := joinData["participantToken"].(string)

	// 4. Validate ReactionAccepted schema
	reactionSchema := doc.Components.Schemas["ReactionAccepted"].Value
	require.NotNil(t, reactionSchema)

	reactBody, _ := json.Marshal(gen.ReactionRequest{Emoji: "👏"})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/rooms/"+roomCode+"/reactions", bytes.NewReader(reactBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Participant-Token", pToken)
	client := &http.Client{}
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	var reactData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&reactData)
	require.NoError(t, err)

	err = reactionSchema.VisitJSON(reactData)
	assert.NoError(t, err, "ReactionAccepted response must match schema")

	// 5. Validate Summary schema
	summarySchema := doc.Components.Schemas["Summary"].Value
	require.NotNil(t, summarySchema)

	resp, err = http.Get(server.URL + "/api/v1/rooms/" + roomCode + "/summary?window=1m")
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var sumData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&sumData)
	require.NoError(t, err)

	err = summarySchema.VisitJSON(sumData)
	assert.NoError(t, err, "Summary response must match Summary schema")

	// 6. Validate Problem schema on error
	problemSchema := doc.Components.Schemas["Problem"].Value
	require.NotNil(t, problemSchema)

	resp, err = http.Get(server.URL + "/api/v1/rooms/non-existent-room")
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	var probData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&probData)
	require.NoError(t, err)

	err = problemSchema.VisitJSON(probData)
	assert.NoError(t, err, "404 Problem response must match Problem schema")

	// 7. Validate ClosedRoom schema
	closedSchema := doc.Components.Schemas["ClosedRoom"].Value
	require.NotNil(t, closedSchema)

	closeReq, _ := http.NewRequest(http.MethodPost, server.URL+"/api/v1/rooms/"+roomCode+"/close", nil)
	closeReq.Header.Set("X-Host-Token", hostToken)
	resp, err = client.Do(closeReq)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var closedData map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&closedData)
	require.NoError(t, err)

	err = closedSchema.VisitJSON(closedData)
	assert.NoError(t, err, "ClosedRoom response must match ClosedRoom schema")
	_ = ctx
}
