package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mahiro424/cbs/internal/config"
	"github.com/mahiro424/cbs/internal/storage"
)

type Server struct {
	cfg       config.Config
	routes    map[string]Route
	pathIndex map[string]struct{}
	seq       atomic.Uint64
}

func NewServer(cfg config.Config) *Server {
	s := &Server{cfg: cfg, routes: make(map[string]Route), pathIndex: make(map[string]struct{})}
	for _, route := range AllRoutes() {
		method := strings.ToUpper(route.Method)
		route.Method = method
		s.routes[method+" "+route.Path] = route
		s.pathIndex[route.Path] = struct{}{}
	}
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := s.requestID(r)
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "????", RequestID: requestID, Data: map[string]any{
			"app":     s.cfg.AppName,
			"listen":  s.cfg.ListenAddress(),
			"redis":   storage.CheckRedis(context.Background(), s.cfg),
			"routes":  len(AllRoutes()),
			"mode":    "mock-first",
			"version": "0.1.0",
		}})
		return
	}
	key := strings.ToUpper(r.Method) + " " + r.URL.Path
	route, ok := s.routes[key]
	if !ok {
		if _, exists := s.pathIndex[r.URL.Path]; exists {
			s.write(w, http.StatusMethodNotAllowed, Envelope{Success: false, Code: "method_not_allowed", Message: "???????", RequestID: requestID, Data: map[string]any{"path": r.URL.Path, "method": r.Method}})
			return
		}
		s.write(w, http.StatusNotFound, Envelope{Success: false, Code: "route_not_found", Message: "?????", RequestID: requestID, Data: map[string]any{"path": r.URL.Path, "method": r.Method}})
		return
	}
	if route.Path == "/Login/GetQR" {
		s.handleLoginGetQR(w, r, requestID)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: false, Code: "not_implemented", Message: "?????????", RequestID: requestID, Data: map[string]any{"path": route.Path, "method": route.Method, "module": route.Module, "operation": route.Operation}})
}

func (s *Server) requestID(r *http.Request) string {
	if v := strings.TrimSpace(r.Header.Get("X-Request-Id")); v != "" {
		return v
	}
	n := s.seq.Add(1)
	return fmt.Sprintf("req-%d-%06d", time.Now().UnixNano(), n)
}

func (s *Server) write(w http.ResponseWriter, status int, body Envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type getQRRequest struct {
	DeviceID   string `json:"DeviceID"`
	DeviceName string `json:"DeviceName"`
	Type       string `json:"Type"`
	Proxy      any    `json:"Proxy,omitempty"`
}

func (s *Server) handleLoginGetQR(w http.ResponseWriter, r *http.Request, requestID string) {
	var req getQRRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: err.Error(), RequestID: requestID})
		return
	}
	seed := strings.Join([]string{req.DeviceID, req.DeviceName, req.Type}, "|")
	if strings.Trim(seed, "|") == "" {
		seed = "anonymous-device"
	}
	sum := sha256.Sum256([]byte(seed))
	uuid := "mock-" + hex.EncodeToString(sum[:])[:24]
	deviceID := strings.TrimSpace(req.DeviceID)
	if deviceID == "" {
		deviceID = "mock-device"
	}
	deviceName := strings.TrimSpace(req.DeviceName)
	if deviceName == "" {
		deviceName = "mock-device-name"
	}
	deviceType := strings.TrimSpace(req.Type)
	if deviceType == "" {
		deviceType = "ipad"
	}
	data := map[string]any{
		"mode":        "mock",
		"uuid":        uuid,
		"qr_url":      "mock://login/" + uuid,
		"cache_key":   "login:mock:" + uuid,
		"device_id":   deviceID,
		"device_name": deviceName,
		"type":        deviceType,
		"stages": []string{
			"parse_request",
			"build_login_context",
			"prepare_device_profile",
			"hybrid_ecdh_ios_pack_placeholder",
			"mock_network_response",
			"login_state_placeholder",
		},
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock ????????", RequestID: requestID, Data: data})
}

func decodeJSON(body io.Reader, out any) error {
	if body == nil {
		return nil
	}
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return fmt.Errorf("JSON ??????%w", err)
	}
	return nil
}
