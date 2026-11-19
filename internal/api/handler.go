package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/coder/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/termofeed/termofeed-back/internal/api/gen"
	"github.com/termofeed/termofeed-back/internal/config"
	"github.com/termofeed/termofeed-back/internal/ratelimit"
	"github.com/termofeed/termofeed-back/internal/room"
	"github.com/termofeed/termofeed-back/internal/ws"
)

type Handler struct {
	service *room.Service
	wsHub   *ws.Hub
	config  *config.Config
	logger  *slog.Logger
	metrics *Metrics
}

func NewHandler(
	svc *room.Service,
	hub *ws.Hub,
	cfg *config.Config,
	logger *slog.Logger,
	reg prometheus.Registerer,
) *Handler {
	var metrics *Metrics
	if reg != nil {
		metrics = NewMetrics(reg)
		hub.SetCallbacks(
			func(code string, count int) {
				metrics.ActiveConnections.Inc()
			},
			func(code string, count int) {
				metrics.ActiveConnections.Dec()
			},
		)
	}

	return &Handler{
		service: svc,
		wsHub:   hub,
		config:  cfg,
		logger:  logger,
		metrics: metrics,
	}
}

func (h *Handler) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req gen.CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondProblem(w, http.StatusBadRequest, "VALIDATION_ERROR", "Corpo da requisição inválido.", nil, nil)
		return
	}

	resp, err := h.service.CreateRoom(r.Context(), req)
	if err != nil {
		RespondError(w, err)
		return
	}

	if h.metrics != nil {
		h.metrics.ActiveRooms.Inc()
	}

	RespondJSON(w, http.StatusCreated, resp)
}

func (h *Handler) GetRoom(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	roomModel, err := h.service.GetRoom(r.Context(), code)
	if err != nil {
		RespondError(w, err)
		return
	}

	RespondJSON(w, http.StatusOK, roomModel)
}

func (h *Handler) JoinRoom(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")

	var req *gen.JoinRequest
	if r.Body != nil {
		var body gen.JoinRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			req = &body
		} else if !errors.Is(err, io.EOF) {
			RespondProblem(w, http.StatusBadRequest, "VALIDATION_ERROR", "Corpo da requisição inválido.", nil, nil)
			return
		}
	}

	resp, err := h.service.JoinRoom(r.Context(), code, req)
	if err != nil {
		RespondError(w, err)
		return
	}

	RespondJSON(w, http.StatusCreated, resp)
}

func (h *Handler) SendReaction(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	token := r.Header.Get("X-Participant-Token")

	var req gen.ReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondProblem(w, http.StatusBadRequest, "VALIDATION_ERROR", "Corpo da requisição inválido.", nil, nil)
		return
	}

	resp, err := h.service.SendReaction(r.Context(), code, token, req)
	if err != nil {
		RespondError(w, err)
		return
	}

	if h.metrics != nil {
		h.metrics.ReactionsTotal.Inc()
	}

	RespondJSON(w, http.StatusAccepted, resp)
}

func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	windowQuery := r.URL.Query().Get("window")

	var windowParam *gen.GetSummaryParamsWindow
	if windowQuery != "" {
		param := gen.GetSummaryParamsWindow(windowQuery)
		if !param.Valid() {
			RespondProblem(w, http.StatusBadRequest, "VALIDATION_ERROR", "Janela inválida. Use '30s', '1m', '5m' ou 'all'.", nil, nil)
			return
		}
		windowParam = &param
	}

	summaryResp, err := h.service.GetSummary(r.Context(), code, windowParam)
	if err != nil {
		RespondError(w, err)
		return
	}

	RespondJSON(w, http.StatusOK, summaryResp)
}

func (h *Handler) ExtendRoom(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	hostToken := r.Header.Get("X-Host-Token")

	var req gen.ExtendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondProblem(w, http.StatusBadRequest, "VALIDATION_ERROR", "Corpo da requisição inválido.", nil, nil)
		return
	}

	updatedRoom, err := h.service.ExtendRoom(r.Context(), code, hostToken, req)
	if err != nil {
		RespondError(w, err)
		return
	}

	RespondJSON(w, http.StatusOK, updatedRoom)
}

func (h *Handler) CloseRoom(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	hostToken := r.Header.Get("X-Host-Token")

	closedResp, err := h.service.CloseRoom(r.Context(), code, hostToken)
	if err != nil {
		RespondError(w, err)
		return
	}

	if h.metrics != nil {
		h.metrics.ActiveRooms.Dec()
	}

	RespondJSON(w, http.StatusOK, closedResp)
}

func (h *Handler) ConnectWebSocket(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	token := r.URL.Query().Get("token")
	if token == "" {
		RespondProblem(w, http.StatusUnauthorized, "UNAUTHORIZED", "Token obrigatório.", nil, nil)
		return
	}

	participant, err := h.service.AuthenticateParticipant(r.Context(), code, token)
	if err != nil {
		RespondError(w, err)
		return
	}

	originPatterns := h.config.AllowedOrigins
	if len(originPatterns) == 0 {
		originPatterns = []string{"*"}
	}

	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: originPatterns,
	})
	if err != nil {
		h.logger.Debug("websocket handshake failed", "err", err)
		return
	}

	clientConn := ws.NewClientConn(h.wsHub, c, code, participant.ID)
	h.wsHub.Register(clientConn)

	go clientConn.WritePump(r.Context())
	clientConn.ReadPump(r.Context())
}

func (h *Handler) Healthz(w http.ResponseWriter, _ *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) Readyz(w http.ResponseWriter, _ *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func BuildRouter(h *Handler, limiter *ratelimit.IPRateLimiter) http.Handler {
	mux := http.NewServeMux()

	// Health and metrics
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.Handle("GET /metrics", promhttp.Handler())

	// API v1 routes
	mux.HandleFunc("POST /api/v1/rooms", h.CreateRoom)
	mux.HandleFunc("GET /api/v1/rooms/{code}", h.GetRoom)
	mux.HandleFunc("POST /api/v1/rooms/{code}/join", h.JoinRoom)
	mux.HandleFunc("POST /api/v1/rooms/{code}/reactions", h.SendReaction)
	mux.HandleFunc("GET /api/v1/rooms/{code}/summary", h.GetSummary)
	mux.HandleFunc("POST /api/v1/rooms/{code}/extend", h.ExtendRoom)
	mux.HandleFunc("POST /api/v1/rooms/{code}/close", h.CloseRoom)
	mux.HandleFunc("GET /api/v1/rooms/{code}/ws", h.ConnectWebSocket)

	var handler http.Handler = mux
	handler = MaxBodySizeMiddleware(8 * 1024)(handler)
	handler = RateLimitMiddleware(limiter, h.config.TrustedProxy)(handler)
	handler = CORSMiddleware(h.config)(handler)
	handler = LoggingAndMetricsMiddleware(h.logger, h.metrics)(handler)

	return handler
}
