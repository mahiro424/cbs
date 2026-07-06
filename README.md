# cbs_rebuild

这是一个不依赖原 `main.exe` 的 Go 版五端算法服务复刻项目。

## 第一轮范围

- 服务可启动。
- 读取兼容 `conf/app.conf` 的配置。
- 检查 Redis 可用性，不可用时给出清晰日志。
- 注册现有 Swagger 中的 142 个接口路由。
- 未实现接口返回统一 `not_implemented` JSON 响应。
- `/Login/GetQR`、`/Login/CheckQR`、`/Login/62data`、`/Login/A16Data` 提供 mock 链路，并输出协议占位、登录态和样本路径。
- 提供 AES、HKDF、CRC、zlib、ECDH 等基础算法接口和测试。

## 运行

```powershell
go test ./...
go run ./cmd/server -config conf/app.conf
```

健康检查：

```powershell
Invoke-RestMethod http://127.0.0.1:7056/healthz
```

GetQR mock：

```powershell
Invoke-RestMethod -Method Post http://127.0.0.1:7056/Login/GetQR -ContentType 'application/json' -Body '{"DeviceID":"dev-001","DeviceName":"测试设备","Type":"ipad"}'
```

CheckQR mock：

```powershell
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/CheckQR?uuid=<uuid>' -Body '{}'
```

62data / A16Data mock：

```powershell
Invoke-RestMethod -Method Post http://127.0.0.1:7056/Login/62data -ContentType 'application/json' -Body '{"Data62":"mock-62-data","DeviceID":"iphone-001","DeviceName":"62样本设备","Wxid":"wxid_62"}'
Invoke-RestMethod -Method Post http://127.0.0.1:7056/Login/A16Data -ContentType 'application/json' -Body '{"A16":"mock-a16-data","DeviceID":"android-001","DeviceName":"A16样本设备","Wxid":"wxid_a16"}'
```

## 当前登录 mock 链路

`/Login/GetQR` 当前会经过一条可验证的二维码 mock 登录链路：

1. 解析设备请求。
2. 生成 Hybrid ECDH iOS 协议占位摘要。
3. 生成 mock 二维码响应。
4. 保存内存登录态。
5. 将请求、协议占位、mock 响应和登录态摘要落盘为 JSON 样本。
6. 返回 `uuid`、`cache_key`、`protocol`、`login_state` 和 `sample_path`。

`/Login/CheckQR` 会读取 `/Login/GetQR` 生成的 `uuid` 登录态，在 mock 模式下返回稳定的 `waiting_scan` 状态，写入最近一次检查样本，并让 `/Login/GetCacheInfo` 继续读回同一登录态。

`/Login/62data` 与 `/Login/A16Data` 会分别生成 iOS / Android 的协议占位摘要、mock 登录响应、登录态和样本文件，供后续真实协议与样本对拍替换。

查询登录态：

```powershell
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/GetCacheInfo?uuid=<uuid>' -Body '{}'
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/GetCacheInfo?cache_key=<cache_key>' -Body '{}'
```

默认样本目录：

```text
.scratch/samples
```
