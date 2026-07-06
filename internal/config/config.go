package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config 保存与原 main.exe app.conf 兼容的核心配置。
type Config struct {
	AppName         string
	HTTPAddr        string
	HTTPPort        int
	RunMode         string
	ViewsPath       string
	AutoRender      bool
	CopyRequestBody bool
	EnableDocs      bool
	RedisLink       string
	RedisPass       string
	RedisDBNum      int
	OCRURL          string
	SessionOn       bool
	SessionName     string
	ServerName      string
	LongLinkEnabled bool
	SampleDir       string
	LoginStateStore string
}

// Default 返回原服务常见默认值，确保缺少配置文件时仍可启动。
func Default() Config {
	return Config{
		AppName:         "wxapi",
		HTTPAddr:        "0.0.0.0",
		HTTPPort:        7056,
		RunMode:         "dev",
		ViewsPath:       "template",
		AutoRender:      false,
		CopyRequestBody: true,
		EnableDocs:      true,
		RedisLink:       "127.0.0.1:6379",
		RedisPass:       "",
		RedisDBNum:      7,
		OCRURL:          "",
		SessionOn:       true,
		SessionName:     "wxapi",
		ServerName:      "wxapi",
		LongLinkEnabled: true,
		SampleDir:       ".scratch/samples",
		LoginStateStore: "memory",
	}
}

// ListenAddress 返回 HTTP 监听地址。
func (c Config) ListenAddress() string {
	addr := strings.TrimSpace(c.HTTPAddr)
	if addr == "" {
		addr = "0.0.0.0"
	}
	port := c.HTTPPort
	if port == 0 {
		port = 7056
	}
	return fmt.Sprintf("%s:%d", addr, port)
}

// LoadFile 读取 Beego 风格 key=value 配置。
func LoadFile(path string) (Config, error) {
	cfg := Default()
	f, err := os.Open(path)
	if err != nil {
		return cfg, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:idx]))
		value := cleanValue(line[idx+1:])
		apply(&cfg, key, value)
	}
	if err := scanner.Err(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func cleanValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Trim(v, "\"")
	return v
}

func apply(c *Config, key, value string) {
	switch key {
	case "appname":
		c.AppName = value
	case "httpaddr":
		c.HTTPAddr = value
	case "httpport":
		if n, err := strconv.Atoi(value); err == nil {
			c.HTTPPort = n
		}
	case "runmode":
		c.RunMode = value
	case "viewspath":
		c.ViewsPath = value
	case "autorender":
		c.AutoRender = parseBool(value)
	case "copyrequestbody":
		c.CopyRequestBody = parseBool(value)
	case "enabledocs":
		c.EnableDocs = parseBool(value)
	case "redislink":
		c.RedisLink = value
	case "redispass":
		c.RedisPass = value
	case "redisdbnum":
		if n, err := strconv.Atoi(value); err == nil {
			c.RedisDBNum = n
		}
	case "ocrurl":
		c.OCRURL = value
	case "sessionon":
		c.SessionOn = parseBool(value)
	case "sessionname":
		c.SessionName = value
	case "servername":
		c.ServerName = value
	case "longlinkenabled":
		c.LongLinkEnabled = parseBool(value)
	case "sampledir":
		c.SampleDir = value
	case "loginstatestore", "login_state_store":
		c.LoginStateStore = strings.ToLower(strings.TrimSpace(value))
	}
}

func parseBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
