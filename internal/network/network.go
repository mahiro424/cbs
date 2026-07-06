package network

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNetworkConfig       = errors.New("network config invalid")
	ErrRealNetworkNotReady = errors.New("real network mode not ready")
)

type Config struct {
	Mode string
}

type Client interface {
	Send(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	Operation string
	LoginKind string
	Platform  string
	Payload   []byte
	Metadata  map[string]string
}

type Response struct {
	Mode          string
	Operation     string
	LoginKind     string
	Platform      string
	Stage         string
	PayloadSHA256 string
	PayloadLength int
	Metadata      map[string]string
}

func (r Response) ToMap() map[string]any {
	m := map[string]any{
		"mode":           r.Mode,
		"operation":      r.Operation,
		"login_kind":     r.LoginKind,
		"platform":       r.Platform,
		"stage":          r.Stage,
		"payload_sha256": r.PayloadSHA256,
		"payload_length": r.PayloadLength,
	}
	if len(r.Metadata) > 0 {
		metadata := make(map[string]string, len(r.Metadata))
		for key, value := range r.Metadata {
			metadata[key] = value
		}
		m["metadata"] = metadata
	}
	return m
}

func ResolveMode(value string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		mode = "mock"
	}
	switch mode {
	case "mock", "real":
		return mode, nil
	default:
		return mode, fmt.Errorf("%w: 不支持的网络模式 %q", ErrNetworkConfig, value)
	}
}

func NewClient(cfg Config) (Client, string, error) {
	mode, err := ResolveMode(cfg.Mode)
	if err != nil {
		return errorClient{err: err}, mode, err
	}
	switch mode {
	case "real":
		return realClient{}, mode, nil
	default:
		return mockClient{}, mode, nil
	}
}

type mockClient struct{}

func (c mockClient) Send(ctx context.Context, req Request) (Response, error) {
	if err := contextError(ctx); err != nil {
		return Response{}, err
	}
	sum := sha256.Sum256(req.Payload)
	return Response{
		Mode:          "mock",
		Operation:     strings.TrimSpace(req.Operation),
		LoginKind:     strings.TrimSpace(req.LoginKind),
		Platform:      strings.TrimSpace(req.Platform),
		Stage:         "mock_network_response",
		PayloadSHA256: hex.EncodeToString(sum[:]),
		PayloadLength: len(req.Payload),
		Metadata:      cloneMetadata(req.Metadata),
	}, nil
}

type realClient struct{}

func (c realClient) Send(ctx context.Context, req Request) (Response, error) {
	if err := contextError(ctx); err != nil {
		return Response{}, err
	}
	return Response{}, fmt.Errorf("%w: MMTLS/HTTP sender 尚未接入", ErrRealNetworkNotReady)
}

type errorClient struct {
	err error
}

func (c errorClient) Send(ctx context.Context, req Request) (Response, error) {
	return Response{}, c.err
}

func cloneMetadata(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}
