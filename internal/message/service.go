package message

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mahiro424/cbs/internal/network"
	"github.com/mahiro424/cbs/internal/protocol"
	"github.com/mahiro424/cbs/internal/storage"
)

var (
	ErrLoginStateNotFound = errors.New("message login state not found")
	ErrSessionLoggedOut   = errors.New("message session logged out")
	ErrProtocolPack       = errors.New("message protocol pack failed")
	ErrSamplePath         = errors.New("message sample path failed")
	ErrSampleWrite        = errors.New("message sample write failed")
	ErrNetwork            = errors.New("message network send failed")
	ErrStateStore         = errors.New("message login state store failed")
)

type Dependencies struct {
	States    storage.LoginStateStore
	Network   network.Client
	SampleDir string
	Now       func() time.Time
}

type Service struct {
	states    storage.LoginStateStore
	network   network.Client
	sampleDir string
	now       func() time.Time
}

func NewService(deps Dependencies) *Service {
	states := deps.States
	if states == nil {
		states = storage.NewMemoryLoginStateStore()
	}
	netClient := deps.Network
	if netClient == nil {
		netClient, _, _ = network.NewClient(network.Config{})
	}
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &Service{
		states:    states,
		network:   netClient,
		sampleDir: deps.SampleDir,
		now:       now,
	}
}

type SendTextRequest struct {
	Wxid    string
	ToWxid  string
	Content string
	Type    int64
	At      string
}

type SyncRequest struct {
	Wxid    string
	Scene   int32
	Synckey string
}

type SendTextResult struct {
	Status        string
	MessageID     string
	Wxid          string
	ToWxid        string
	Content       string
	ContentLength int
	Type          int64
	At            string
	SentAt        time.Time
	Protocol      map[string]any
	Network       map[string]any
	LoginState    storage.LoginState
	SamplePath    string
	Stages        []string
}

type SyncResult struct {
	Status      string
	SyncID      string
	Wxid        string
	Scene       int32
	SyncKey     string
	NextSyncKey string
	SyncedAt    time.Time
	Protocol    map[string]any
	Network     map[string]any
	LoginState  storage.LoginState
	SamplePath  string
	Stages      []string
}

func (r SendTextResult) ResponseData() map[string]any {
	data := map[string]any{
		"status":         r.Status,
		"message_id":     r.MessageID,
		"wxid":           r.Wxid,
		"to_wxid":        r.ToWxid,
		"content_length": r.ContentLength,
		"type":           r.Type,
		"sent_at":        r.SentAt.Format(time.RFC3339Nano),
		"protocol":       r.Protocol,
		"network":        r.Network,
		"login_state":    r.LoginState.ToMap(),
		"sample_path":    r.SamplePath,
		"stages":         r.Stages,
	}
	if r.At != "" {
		data["at"] = r.At
	}
	return data
}

func (r SyncResult) ResponseData() map[string]any {
	return map[string]any{
		"status":       r.Status,
		"sync_id":      r.SyncID,
		"wxid":         r.Wxid,
		"scene":        r.Scene,
		"synckey":      r.SyncKey,
		"next_synckey": r.NextSyncKey,
		"synced_at":    r.SyncedAt.Format(time.RFC3339Nano),
		"protocol":     r.Protocol,
		"network":      r.Network,
		"login_state":  r.LoginState.ToMap(),
		"sample_path":  r.SamplePath,
		"stages":       r.Stages,
	}
}

func (s *Service) SendText(ctx context.Context, req SendTextRequest) (SendTextResult, error) {
	wxid := strings.TrimSpace(req.Wxid)
	toWxid := strings.TrimSpace(req.ToWxid)
	content := req.Content
	at := strings.TrimSpace(req.At)
	state, ok, err := s.states.GetByWxid(ctx, wxid)
	if err != nil {
		return SendTextResult{}, fmt.Errorf("%w: %v", ErrStateStore, err)
	}
	if !ok {
		return SendTextResult{}, fmt.Errorf("%w: %s", ErrLoginStateNotFound, wxid)
	}
	if state.SessionState == "logged_out" {
		return SendTextResult{LoginState: state}, fmt.Errorf("%w: %s", ErrSessionLoggedOut, wxid)
	}
	sentAt := s.now().UTC()
	messageID := messageID(wxid, toWxid, content, req.Type, at, sentAt)
	payload := map[string]any{
		"wxid":       wxid,
		"to_wxid":    toWxid,
		"content":    content,
		"type":       req.Type,
		"at":         at,
		"message_id": messageID,
		"login_uuid": state.UUID,
		"sent_at":    sentAt.Format(time.RFC3339Nano),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return SendTextResult{}, fmt.Errorf("%w: %v", ErrProtocolPack, err)
	}
	packed, debug, err := protocol.PackBusinessPacket(protocol.BusinessPacket{
		Operation: "Msg.SendTxt",
		Payload:   payloadBytes,
		Flags:     1,
	})
	if err != nil {
		return SendTextResult{}, fmt.Errorf("%w: %v", ErrProtocolPack, err)
	}
	sum := sha256.Sum256(payloadBytes)
	protocolTrace := map[string]any{
		"pack_kind":      "business_packet_mock",
		"operation":      "Msg.SendTxt",
		"payload_sha256": hex.EncodeToString(sum[:]),
		"payload_length": len(payloadBytes),
		"packed_hex":     protocol.ToHex(packed),
		"debug":          debug,
	}
	networkResp, err := s.network.Send(ctx, network.Request{
		Operation: "Msg.SendTxt",
		LoginKind: state.LoginKind,
		Platform:  messagePlatform(state.Type),
		Payload:   packed,
		Metadata: map[string]string{
			"wxid":       wxid,
			"to_wxid":    toWxid,
			"message_id": messageID,
		},
	})
	if err != nil {
		return SendTextResult{}, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	networkTrace := networkResp.ToMap()
	samplePath, err := sampleFilePath(s.sampleDir, messageID)
	if err != nil {
		return SendTextResult{}, fmt.Errorf("%w: %v", ErrSamplePath, err)
	}
	mockResponse := map[string]any{
		"status":     "mock_sent",
		"message_id": messageID,
		"wxid":       wxid,
		"to_wxid":    toWxid,
		"sent_at":    sentAt.Format(time.RFC3339Nano),
	}
	sample := map[string]any{
		"request": map[string]any{
			"wxid":    wxid,
			"to_wxid": toWxid,
			"content": content,
			"type":    req.Type,
			"at":      at,
		},
		"protocol":      protocolTrace,
		"network":       networkTrace,
		"mock_response": mockResponse,
		"login_state":   state.ToMap(),
	}
	if err := writeSample(samplePath, sample); err != nil {
		return SendTextResult{}, fmt.Errorf("%w: %v", ErrSampleWrite, err)
	}
	return SendTextResult{
		Status:        "mock_sent",
		MessageID:     messageID,
		Wxid:          wxid,
		ToWxid:        toWxid,
		Content:       content,
		ContentLength: utf8.RuneCountInString(content),
		Type:          req.Type,
		At:            at,
		SentAt:        sentAt,
		Protocol:      protocolTrace,
		Network:       networkTrace,
		LoginState:    state,
		SamplePath:    samplePath,
		Stages: []string{
			"parse_request",
			"load_wxid_login_state",
			"business_packet_pack",
			"mock_network_response",
			"write_sample",
		},
	}, nil
}

func (s *Service) Sync(ctx context.Context, req SyncRequest) (SyncResult, error) {
	wxid := strings.TrimSpace(req.Wxid)
	syncKey := strings.TrimSpace(req.Synckey)
	state, ok, err := s.states.GetByWxid(ctx, wxid)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrStateStore, err)
	}
	if !ok {
		return SyncResult{}, fmt.Errorf("%w: %s", ErrLoginStateNotFound, wxid)
	}
	if state.SessionState == "logged_out" {
		return SyncResult{LoginState: state}, fmt.Errorf("%w: %s", ErrSessionLoggedOut, wxid)
	}
	syncedAt := s.now().UTC()
	syncID := syncID(wxid, req.Scene, syncKey, syncedAt)
	nextSyncKey := nextSyncKey(state.UUID, wxid, req.Scene, syncKey, syncedAt)
	payload := map[string]any{
		"wxid":         wxid,
		"scene":        req.Scene,
		"synckey":      syncKey,
		"next_synckey": nextSyncKey,
		"sync_id":      syncID,
		"login_uuid":   state.UUID,
		"synced_at":    syncedAt.Format(time.RFC3339Nano),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrProtocolPack, err)
	}
	packed, debug, err := protocol.PackBusinessPacket(protocol.BusinessPacket{
		Operation: "Msg.Sync",
		Payload:   payloadBytes,
		Flags:     1,
	})
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrProtocolPack, err)
	}
	sum := sha256.Sum256(payloadBytes)
	protocolTrace := map[string]any{
		"pack_kind":      "business_packet_mock",
		"operation":      "Msg.Sync",
		"payload_sha256": hex.EncodeToString(sum[:]),
		"payload_length": len(payloadBytes),
		"packed_hex":     protocol.ToHex(packed),
		"debug":          debug,
	}
	networkResp, err := s.network.Send(ctx, network.Request{
		Operation: "Msg.Sync",
		LoginKind: state.LoginKind,
		Platform:  messagePlatform(state.Type),
		Payload:   packed,
		Metadata: map[string]string{
			"wxid":         wxid,
			"sync_id":      syncID,
			"next_synckey": nextSyncKey,
		},
	})
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrNetwork, err)
	}
	networkTrace := networkResp.ToMap()
	samplePath, err := sampleFilePath(s.sampleDir, syncID)
	if err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrSamplePath, err)
	}
	mockResponse := map[string]any{
		"status":       "mock_synced",
		"sync_id":      syncID,
		"wxid":         wxid,
		"scene":        req.Scene,
		"synckey":      syncKey,
		"next_synckey": nextSyncKey,
		"synced_at":    syncedAt.Format(time.RFC3339Nano),
	}
	sample := map[string]any{
		"request": map[string]any{
			"wxid":    wxid,
			"scene":   req.Scene,
			"synckey": syncKey,
		},
		"protocol":      protocolTrace,
		"network":       networkTrace,
		"mock_response": mockResponse,
		"login_state":   state.ToMap(),
	}
	if err := writeSample(samplePath, sample); err != nil {
		return SyncResult{}, fmt.Errorf("%w: %v", ErrSampleWrite, err)
	}
	return SyncResult{
		Status:      "mock_synced",
		SyncID:      syncID,
		Wxid:        wxid,
		Scene:       req.Scene,
		SyncKey:     syncKey,
		NextSyncKey: nextSyncKey,
		SyncedAt:    syncedAt,
		Protocol:    protocolTrace,
		Network:     networkTrace,
		LoginState:  state,
		SamplePath:  samplePath,
		Stages: []string{
			"parse_request",
			"load_wxid_login_state",
			"business_packet_pack",
			"mock_network_response",
			"write_sample",
		},
	}, nil
}

func messageID(wxid, toWxid, content string, msgType int64, at string, sentAt time.Time) string {
	seed := strings.Join([]string{wxid, toWxid, content, fmt.Sprintf("%d", msgType), at, sentAt.Format(time.RFC3339Nano)}, "|")
	sum := sha256.Sum256([]byte(seed))
	return "mock-msg-" + hex.EncodeToString(sum[:])[:24]
}

func syncID(wxid string, scene int32, syncKey string, syncedAt time.Time) string {
	seed := strings.Join([]string{wxid, fmt.Sprintf("%d", scene), syncKey, syncedAt.Format(time.RFC3339Nano)}, "|")
	sum := sha256.Sum256([]byte(seed))
	return "mock-sync-" + hex.EncodeToString(sum[:])[:24]
}

func nextSyncKey(uuid, wxid string, scene int32, syncKey string, syncedAt time.Time) string {
	seed := strings.Join([]string{uuid, wxid, fmt.Sprintf("%d", scene), syncKey, syncedAt.Format(time.RFC3339Nano)}, "|")
	sum := sha256.Sum256([]byte(seed))
	return "next-" + hex.EncodeToString(sum[:])[:24]
}

func messagePlatform(deviceType string) string {
	switch strings.ToLower(strings.TrimSpace(deviceType)) {
	case "iphone", "ipad", "ios":
		return "ios"
	case "android":
		return "android"
	default:
		return strings.TrimSpace(deviceType)
	}
}

func sampleFilePath(sampleDir, messageID string) (string, error) {
	if strings.TrimSpace(sampleDir) == "" {
		sampleDir = ".scratch/samples"
	}
	absDir, err := filepath.Abs(sampleDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(absDir, messageID+".json"), nil
}

func writeSample(path string, sample map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(payload, '\n'), 0o644)
}
