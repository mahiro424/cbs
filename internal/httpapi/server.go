package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mahiro424/cbs/internal/config"
	loginpkg "github.com/mahiro424/cbs/internal/login"
	messagepkg "github.com/mahiro424/cbs/internal/message"
	"github.com/mahiro424/cbs/internal/network"
	"github.com/mahiro424/cbs/internal/storage"
)

type Server struct {
	cfg       config.Config
	routes    map[string]Route
	pathIndex map[string]struct{}
	states    storage.LoginStateStore
	stateMode string
	stateErr  error
	network   network.Client
	netMode   string
	netErr    error
	login     *loginpkg.Service
	message   *messagepkg.Service
	seq       atomic.Uint64
}

func NewServer(cfg config.Config) *Server {
	states, stateMode, stateErr := storage.NewLoginStateStoreFromConfig(cfg)
	netClient, netMode, netErr := network.NewClient(network.Config{Mode: cfg.NetworkMode})
	s := &Server{cfg: cfg, routes: make(map[string]Route), pathIndex: make(map[string]struct{}), states: states, stateMode: stateMode, stateErr: stateErr, network: netClient, netMode: netMode, netErr: netErr}
	s.login = loginpkg.NewService(loginpkg.Dependencies{States: states, Network: netClient, SampleDir: cfg.SampleDir})
	s.message = messagepkg.NewService(messagepkg.Dependencies{States: states, Network: netClient, SampleDir: cfg.SampleDir})
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
		s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "服务正常", RequestID: requestID, Data: map[string]any{
			"app":               s.cfg.AppName,
			"listen":            s.cfg.ListenAddress(),
			"redis":             storage.CheckRedis(context.Background(), s.cfg),
			"login_state_store": s.loginStateStoreSummary(),
			"network":           s.networkSummary(),
			"routes":            len(AllRoutes()),
			"mode":              "mock-first",
			"version":           "0.1.0",
		}})
		return
	}
	key := strings.ToUpper(r.Method) + " " + r.URL.Path
	route, ok := s.routes[key]
	if !ok {
		if _, exists := s.pathIndex[r.URL.Path]; exists {
			s.write(w, http.StatusMethodNotAllowed, Envelope{Success: false, Code: "method_not_allowed", Message: "请求方法不匹配", RequestID: requestID, Data: map[string]any{"path": r.URL.Path, "method": r.Method}})
			return
		}
		s.write(w, http.StatusNotFound, Envelope{Success: false, Code: "route_not_found", Message: "路由不存在", RequestID: requestID, Data: map[string]any{"path": r.URL.Path, "method": r.Method}})
		return
	}
	if route.Path == "/Login/GetQR" {
		s.handleLoginGetQR(w, r, requestID)
		return
	}
	if route.Path == "/Login/CheckQR" {
		s.handleLoginCheckQR(w, r, requestID)
		return
	}
	if route.Path == "/Login/62data" {
		s.handleLoginData62(w, r, requestID)
		return
	}
	if route.Path == "/Login/A16Data" {
		s.handleLoginA16Data(w, r, requestID)
		return
	}
	if route.Path == "/Login/GetCacheInfo" {
		s.handleLoginGetCacheInfo(w, r, requestID)
		return
	}
	if route.Path == "/Login/Newinit" {
		s.handleLoginNewinit(w, r, requestID)
		return
	}
	if route.Path == "/Login/HeartBeat" {
		s.handleLoginHeartBeat(w, r, requestID)
		return
	}
	if route.Path == "/Login/Get62Data" {
		s.handleLoginGet62Data(w, r, requestID)
		return
	}
	if route.Path == "/Login/GetA16Data" {
		s.handleLoginGetA16Data(w, r, requestID)
		return
	}
	if route.Path == "/Login/LogOut" {
		s.handleLoginLogOut(w, r, requestID)
		return
	}
	if route.Path == "/Msg/SendTxt" {
		s.handleMsgSendTxt(w, r, requestID)
		return
	}
	if route.Path == "/Msg/Sync" {
		s.handleMsgSync(w, r, requestID)
		return
	}
	if route.Path == "/Msg/Revoke" {
		s.handleMsgRevoke(w, r, requestID)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: false, Code: "not_implemented", Message: "接口已接入但未实现", RequestID: requestID, Data: map[string]any{"path": route.Path, "method": route.Method, "module": route.Module, "operation": route.Operation}})
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

func (s *Server) loginStateStoreSummary() map[string]any {
	summary := map[string]any{
		"mode": s.stateMode,
	}
	if s.stateErr != nil {
		summary["available"] = false
		summary["message"] = s.stateErr.Error()
		return summary
	}
	summary["available"] = true
	return summary
}

func (s *Server) networkSummary() map[string]any {
	summary := map[string]any{
		"mode": s.netMode,
	}
	if s.netErr != nil {
		summary["available"] = false
		summary["message"] = s.netErr.Error()
		return summary
	}
	summary["available"] = true
	return summary
}

func (s *Server) writeLoginStateStoreError(w http.ResponseWriter, requestID string, err error) {
	s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "login_state_store_error", Message: err.Error(), RequestID: requestID})
}

func (s *Server) writeNetworkError(w http.ResponseWriter, requestID string, err error) {
	s.write(w, http.StatusBadGateway, Envelope{Success: false, Code: "network_error", Message: err.Error(), RequestID: requestID})
}

func (s *Server) writeLoginServiceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, network.ErrRealNetworkNotReady), errors.Is(err, network.ErrNetworkConfig):
		s.writeNetworkError(w, requestID, err)
	case errors.Is(err, loginpkg.ErrLoginStateNotFound):
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "cache_not_found", Message: "未找到二维码登录态", RequestID: requestID})
	case errors.Is(err, loginpkg.ErrWxidLoginStateNotFound):
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "cache_not_found", Message: "未找到 wxid 登录态", RequestID: requestID})
	case errors.Is(err, loginpkg.ErrSessionLoggedOut):
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "session_logged_out", Message: "登录态已退出", RequestID: requestID})
	case errors.Is(err, loginpkg.ErrUnsupportedLoginKind):
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "unsupported_login_kind", Message: "当前 uuid 不是二维码登录态", RequestID: requestID})
	case errors.Is(err, loginpkg.ErrProtocolPack):
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "protocol_pack_error", Message: err.Error(), RequestID: requestID})
	case errors.Is(err, loginpkg.ErrRiskPlan):
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "risk_algorithm_plan_error", Message: err.Error(), RequestID: requestID})
	case errors.Is(err, loginpkg.ErrSamplePath):
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "sample_path_error", Message: err.Error(), RequestID: requestID})
	case errors.Is(err, loginpkg.ErrSampleWrite):
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "sample_write_error", Message: err.Error(), RequestID: requestID})
	case errors.Is(err, loginpkg.ErrStateStore):
		s.writeLoginStateStoreError(w, requestID, err)
	default:
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "login_error", Message: err.Error(), RequestID: requestID})
	}
}

func (s *Server) writeMessageServiceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, messagepkg.ErrLoginStateNotFound):
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "cache_not_found", Message: "未找到 wxid 登录态", RequestID: requestID})
	case errors.Is(err, messagepkg.ErrSessionLoggedOut):
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "session_logged_out", Message: "登录态已退出", RequestID: requestID})
	case errors.Is(err, messagepkg.ErrStateStore):
		s.writeLoginStateStoreError(w, requestID, err)
	case errors.Is(err, messagepkg.ErrProtocolPack):
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "protocol_pack_error", Message: err.Error(), RequestID: requestID})
	case errors.Is(err, messagepkg.ErrNetwork):
		s.write(w, http.StatusBadGateway, Envelope{Success: false, Code: "network_error", Message: err.Error(), RequestID: requestID})
	case errors.Is(err, messagepkg.ErrSamplePath):
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "sample_path_error", Message: err.Error(), RequestID: requestID})
	case errors.Is(err, messagepkg.ErrSampleWrite):
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "sample_write_error", Message: err.Error(), RequestID: requestID})
	default:
		s.write(w, http.StatusInternalServerError, Envelope{Success: false, Code: "message_error", Message: err.Error(), RequestID: requestID})
	}
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
	result, err := s.login.GetQR(r.Context(), loginpkg.GetQRRequest{
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		Type:       req.Type,
		Proxy:      req.Proxy,
	})
	if err != nil {
		s.writeLoginServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 二维码链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

func (s *Server) handleLoginCheckQR(w http.ResponseWriter, r *http.Request, requestID string) {
	uuid := strings.TrimSpace(r.URL.Query().Get("uuid"))
	if uuid == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 uuid", RequestID: requestID})
		return
	}
	result, err := s.login.CheckQR(r.Context(), loginpkg.CheckQRRequest{UUID: uuid})
	if err != nil {
		s.writeLoginServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 二维码检查链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

type data62LoginRequest struct {
	Data62     string `json:"Data62"`
	DeviceID   string `json:"DeviceID"`
	DeviceName string `json:"DeviceName"`
	Wxid       string `json:"Wxid"`
	Proxy      any    `json:"Proxy,omitempty"`
}

func (s *Server) handleLoginData62(w http.ResponseWriter, r *http.Request, requestID string) {
	var req data62LoginRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: err.Error(), RequestID: requestID})
		return
	}
	result, err := s.login.Import62Data(r.Context(), loginpkg.Import62DataRequest{
		Data62:     req.Data62,
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		Wxid:       req.Wxid,
		Proxy:      req.Proxy,
	})
	if err != nil {
		s.writeLoginServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 62data 登录链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

type a16LoginRequest struct {
	A16        string `json:"A16"`
	DeviceID   string `json:"DeviceID"`
	DeviceName string `json:"DeviceName"`
	Wxid       string `json:"Wxid"`
	Proxy      any    `json:"Proxy,omitempty"`
}

func (s *Server) handleLoginA16Data(w http.ResponseWriter, r *http.Request, requestID string) {
	var req a16LoginRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: err.Error(), RequestID: requestID})
		return
	}
	result, err := s.login.ImportA16Data(r.Context(), loginpkg.ImportA16DataRequest{
		A16:        req.A16,
		DeviceID:   req.DeviceID,
		DeviceName: req.DeviceName,
		Wxid:       req.Wxid,
		Proxy:      req.Proxy,
	})
	if err != nil {
		s.writeLoginServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock A16Data 登录链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

func (s *Server) handleLoginNewinit(w http.ResponseWriter, r *http.Request, requestID string) {
	wxid := strings.TrimSpace(r.URL.Query().Get("wxid"))
	if wxid == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 wxid", RequestID: requestID})
		return
	}
	maxSyncKey := strings.TrimSpace(r.URL.Query().Get("MaxSynckey"))
	currentSyncKey := strings.TrimSpace(r.URL.Query().Get("CurrentSynckey"))
	result, err := s.login.Newinit(r.Context(), loginpkg.NewinitRequest{Wxid: wxid, MaxSyncKey: maxSyncKey, CurrentSyncKey: currentSyncKey})
	if err != nil {
		s.writeLoginServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 登录初始化链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

func (s *Server) handleLoginHeartBeat(w http.ResponseWriter, r *http.Request, requestID string) {
	wxid := strings.TrimSpace(r.URL.Query().Get("wxid"))
	if wxid == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 wxid", RequestID: requestID})
		return
	}
	result, err := s.login.HeartBeat(r.Context(), loginpkg.HeartBeatRequest{Wxid: wxid})
	if err != nil {
		if errors.Is(err, loginpkg.ErrSessionLoggedOut) {
			s.write(w, http.StatusOK, Envelope{Success: false, Code: "session_logged_out", Message: "登录态已退出", RequestID: requestID, Data: map[string]any{"login_state": result.State.ToMap()}})
			return
		}
		s.writeLoginServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 短心跳链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

func (s *Server) handleLoginGet62Data(w http.ResponseWriter, r *http.Request, requestID string) {
	wxid := strings.TrimSpace(r.URL.Query().Get("wxid"))
	if wxid == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 wxid", RequestID: requestID})
		return
	}
	result, err := s.login.Get62Data(r.Context(), loginpkg.ExportLoginDataRequest{Wxid: wxid})
	if err != nil {
		s.writeLoginExportServiceError(w, requestID, result, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 62 数据导出链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

func (s *Server) handleLoginGetA16Data(w http.ResponseWriter, r *http.Request, requestID string) {
	wxid := strings.TrimSpace(r.URL.Query().Get("wxid"))
	if wxid == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 wxid", RequestID: requestID})
		return
	}
	result, err := s.login.GetA16Data(r.Context(), loginpkg.ExportLoginDataRequest{Wxid: wxid})
	if err != nil {
		s.writeLoginExportServiceError(w, requestID, result, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock A16 数据导出链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

func (s *Server) writeLoginExportServiceError(w http.ResponseWriter, requestID string, result loginpkg.ExportLoginDataResult, err error) {
	if errors.Is(err, loginpkg.ErrUnsupportedLoginKind) {
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "unsupported_login_kind", Message: "当前 wxid 登录态不支持该导出类型", RequestID: requestID, Data: result.State.ToMap()})
		return
	}
	s.writeLoginServiceError(w, requestID, err)
}

func (s *Server) handleLoginLogOut(w http.ResponseWriter, r *http.Request, requestID string) {
	wxid := strings.TrimSpace(r.URL.Query().Get("wxid"))
	if wxid == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 wxid", RequestID: requestID})
		return
	}
	result, err := s.login.LogOut(r.Context(), loginpkg.LogOutRequest{Wxid: wxid})
	if err != nil {
		s.writeLoginServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 退出登录链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

func (s *Server) handleLoginGetCacheInfo(w http.ResponseWriter, r *http.Request, requestID string) {
	uuid := strings.TrimSpace(r.URL.Query().Get("uuid"))
	cacheKey := strings.TrimSpace(r.URL.Query().Get("cache_key"))
	if uuid == "" && cacheKey == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 uuid 或 cache_key", RequestID: requestID})
		return
	}
	state, ok, err := s.states.Get(r.Context(), uuid, cacheKey)
	if err != nil {
		s.writeLoginStateStoreError(w, requestID, err)
		return
	}
	if !ok {
		s.write(w, http.StatusOK, Envelope{Success: false, Code: "cache_not_found", Message: "未找到登录态", RequestID: requestID})
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "已读取登录态", RequestID: requestID, Data: state.ToMap()})
}

type sendTextRequest struct {
	Wxid    string `json:"Wxid"`
	ToWxid  string `json:"ToWxid"`
	Content string `json:"Content"`
	Type    int64  `json:"Type"`
	At      string `json:"At"`
}

func (s *Server) handleMsgSendTxt(w http.ResponseWriter, r *http.Request, requestID string) {
	var req sendTextRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: err.Error(), RequestID: requestID})
		return
	}
	if strings.TrimSpace(req.Wxid) == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 Wxid", RequestID: requestID})
		return
	}
	if strings.TrimSpace(req.ToWxid) == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 ToWxid", RequestID: requestID})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 Content", RequestID: requestID})
		return
	}
	result, err := s.message.SendText(r.Context(), messagepkg.SendTextRequest{
		Wxid:    req.Wxid,
		ToWxid:  req.ToWxid,
		Content: req.Content,
		Type:    req.Type,
		At:      req.At,
	})
	if err != nil {
		s.writeMessageServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 文本消息发送链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

type syncRequest struct {
	Wxid    string `json:"Wxid"`
	Scene   int32  `json:"Scene"`
	Synckey string `json:"Synckey"`
}

func (s *Server) handleMsgSync(w http.ResponseWriter, r *http.Request, requestID string) {
	var req syncRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: err.Error(), RequestID: requestID})
		return
	}
	if strings.TrimSpace(req.Wxid) == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 Wxid", RequestID: requestID})
		return
	}
	result, err := s.message.Sync(r.Context(), messagepkg.SyncRequest{
		Wxid:    req.Wxid,
		Scene:   req.Scene,
		Synckey: req.Synckey,
	})
	if err != nil {
		s.writeMessageServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 消息同步链路已跑通", RequestID: requestID, Data: result.ResponseData()})
}

type revokeRequest struct {
	Wxid        string `json:"Wxid"`
	ToUserName  string `json:"ToUserName"`
	NewMsgID    int64  `json:"NewMsgId"`
	ClientMsgID int64  `json:"ClientMsgId"`
	CreateTime  int64  `json:"CreateTime"`
}

func (s *Server) handleMsgRevoke(w http.ResponseWriter, r *http.Request, requestID string) {
	var req revokeRequest
	if err := decodeJSON(r.Body, &req); err != nil {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: err.Error(), RequestID: requestID})
		return
	}
	if strings.TrimSpace(req.Wxid) == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 Wxid", RequestID: requestID})
		return
	}
	if strings.TrimSpace(req.ToUserName) == "" {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 ToUserName", RequestID: requestID})
		return
	}
	if req.NewMsgID == 0 {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 NewMsgId", RequestID: requestID})
		return
	}
	if req.ClientMsgID == 0 {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 ClientMsgId", RequestID: requestID})
		return
	}
	if req.CreateTime == 0 {
		s.write(w, http.StatusBadRequest, Envelope{Success: false, Code: "param_error", Message: "必须提供 CreateTime", RequestID: requestID})
		return
	}
	result, err := s.message.Revoke(r.Context(), messagepkg.RevokeRequest{
		Wxid:        req.Wxid,
		ToUserName:  req.ToUserName,
		NewMsgID:    req.NewMsgID,
		ClientMsgID: req.ClientMsgID,
		CreateTime:  req.CreateTime,
	})
	if err != nil {
		s.writeMessageServiceError(w, requestID, err)
		return
	}
	s.write(w, http.StatusOK, Envelope{Success: true, Code: "ok", Message: "mock 消息撤回链路已跑通", RequestID: requestID, Data: result.ResponseData()})
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
		return fmt.Errorf("JSON 请求体无效：%w", err)
	}
	return nil
}
