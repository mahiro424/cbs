package config_test

import (
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
}

func TestDefaultConfigIsUsableWithoutFile(t *testing.T) {
	cfg := config.Default()
	if cfg.ListenAddress() != "0.0.0.0:7056" {
		t.Fatalf("ListenAddress = %q，期望 0.0.0.0:7056", cfg.ListenAddress())
	}
	if cfg.RunMode != "dev" {
		t.Fatalf("RunMode = %q，期望 dev", cfg.RunMode)
	}
}
