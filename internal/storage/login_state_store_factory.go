package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mahiro424/cbs/internal/config"
)

var ErrLoginStateStoreConfig = errors.New("login state store config invalid")

func ResolveLoginStateStoreMode(value string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" {
		mode = "memory"
	}
	switch mode {
	case "memory", "redis":
		return mode, nil
	default:
		return mode, fmt.Errorf("%w: 不支持的登录态存储模式 %q", ErrLoginStateStoreConfig, value)
	}
}

func NewLoginStateStoreFromConfig(cfg config.Config) (LoginStateStore, string, error) {
	mode, err := ResolveLoginStateStoreMode(cfg.LoginStateStore)
	if err != nil {
		return errorLoginStateStore{err: err}, mode, err
	}
	switch mode {
	case "redis":
		return NewRedisLoginStateStoreFromConfig(cfg), mode, nil
	default:
		return NewMemoryLoginStateStore(), mode, nil
	}
}

type errorLoginStateStore struct {
	err error
}

func (s errorLoginStateStore) Save(ctx context.Context, state LoginState) error {
	return s.err
}

func (s errorLoginStateStore) Get(ctx context.Context, uuid string, cacheKey string) (LoginState, bool, error) {
	return LoginState{}, false, s.err
}

func (s errorLoginStateStore) GetByWxid(ctx context.Context, wxid string) (LoginState, bool, error) {
	return LoginState{}, false, s.err
}
