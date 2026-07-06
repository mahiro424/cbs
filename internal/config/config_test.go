package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mahiro424/cbs/internal/config"
)

func TestLoadAppConfigKeepsOriginalServiceDefaults(t *testing.T) {
	cfg, err := config.LoadFile("../../testdata/app.conf")
	if err != nil {
		t.Fatalf("读取配置失败：%v", err)
	}

	if cfg.AppName != "wxapi" {
		t.Fatalf("AppName = %q，期望 wxapi", cfg.AppName)
	}
	if cfg.HTTPAddr != "0.0.0.0" {
		t.Fatalf("HTTPAddr = %q，期望 0.0.0.0", cfg.HTTPAddr)
	}
	if cfg.HTTPPort != 7056 {
		t.Fatalf("HTTPPort = %d，期望 7056", cfg.HTTPPort)
	}
	if cfg.RedisLink != "127.0.0.1:6379" {
		t.Fatalf("RedisLink = %q，期望 127.0.0.1:6379", cfg.RedisLink)
	}
	if cfg.RedisDBNum != 7 {
		t.Fatalf("RedisDBNum = %d，期望 7", cfg.RedisDBNum)
	}
	if !cfg.LongLinkEnabled {
		t.Fatalf("LongLinkEnabled = false，期望 true")
	}
	if cfg.LoginStateStore != "memory" {
		t.Fatalf("LoginStateStore = %q，期望默认 memory", cfg.LoginStateStore)
	}
	if cfg.NetworkMode != "mock" {
		t.Fatalf("NetworkMode = %q，期望默认 mock", cfg.NetworkMode)
	}
}

func TestDefaultConfigIsUsableWithoutFile(t *testing.T) {
	cfg := config.Default()
	if cfg.ListenAddress() != "0.0.0.0:7056" {
		t.Fatalf("ListenAddress = %q，期望 0.0.0.0:7056", cfg.ListenAddress())
	}
	if cfg.RunMode != "dev" {
		t.Fatalf("RunMode = %q，期望 dev", cfg.RunMode)
	}
	if cfg.LoginStateStore != "memory" {
		t.Fatalf("LoginStateStore = %q，期望 memory", cfg.LoginStateStore)
	}
	if cfg.NetworkMode != "mock" {
		t.Fatalf("NetworkMode = %q，期望 mock", cfg.NetworkMode)
	}
}

func TestLoadAppConfigCanSelectRedisLoginStateStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.conf")
	if err := os.WriteFile(path, []byte("loginstatestore = redis\nredislink = 127.0.0.1:6380\nredisdbnum = 8\n"), 0o644); err != nil {
		t.Fatalf("写入临时配置失败：%v", err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("读取配置失败：%v", err)
	}
	if cfg.LoginStateStore != "redis" {
		t.Fatalf("LoginStateStore = %q，期望 redis", cfg.LoginStateStore)
	}
	if cfg.RedisLink != "127.0.0.1:6380" || cfg.RedisDBNum != 8 {
		t.Fatalf("Redis 配置 = %s / %d，期望读取临时配置", cfg.RedisLink, cfg.RedisDBNum)
	}
}

func TestLoadAppConfigCanSelectNetworkMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.conf")
	if err := os.WriteFile(path, []byte("networkmode = real\n"), 0o644); err != nil {
		t.Fatalf("写入临时配置失败：%v", err)
	}
	cfg, err := config.LoadFile(path)
	if err != nil {
		t.Fatalf("读取配置失败：%v", err)
	}
	if cfg.NetworkMode != "real" {
		t.Fatalf("NetworkMode = %q，期望 real", cfg.NetworkMode)
	}
}
