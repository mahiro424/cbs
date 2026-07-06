# cbs_rebuild

这是一个不依赖原 `main.exe` 的 Go 版五端算法服务复刻项目。

## 第一轮范围

- 服务可启动。
- 读取兼容 `conf/app.conf` 的配置。
- 检查 Redis 可用性，不可用时给出清晰日志。
- 注册现有 Swagger 中的 142 个接口路由。
- 未实现接口返回统一 `not_implemented` JSON 响应。
- `/Login/GetQR`、`/Login/CheckQR`、`/Login/62data`、`/Login/A16Data`、`/Login/Newinit`、`/Login/HeartBeat`、`/Login/Get62Data`、`/Login/GetA16Data`、`/Login/LogOut` 提供 mock 链路，并输出协议占位、登录态和样本路径。
- 提供 AES、HKDF、CRC、zlib、ECDH 等基础算法接口和测试。
- 提供 `internal/protocol` mock-first Pack / Unpack 与 Hybrid ECDH iOS / Android 样本入口，用于后续真实协议对拍替换。

## 运行

Windows / PowerShell 下先进入 UTF-8 会话：

```powershell
. .\scripts\enter-utf8.ps1
```

```powershell
go test ./...
go run ./cmd/server -config conf/app.conf
```

## 编码治理

仓库默认使用 UTF-8。若需要处理父级 `F:\yanjiu` 中的历史 GBK/GB18030 文本，先预演、再转换、最后检查：

```powershell
python .\scripts\normalize_encoding.py F:\yanjiu --limit 20
python .\scripts\normalize_encoding.py F:\yanjiu --write
python .\scripts\check_encoding.py F:\yanjiu
```

转换脚本会把原始字节备份到 `F:\yanjiu\.encoding-backups\<时间戳>\`，再将文本写回为 UTF-8。

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

登录后 Newinit / HeartBeat mock：

```powershell
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/Newinit?wxid=<wxid>' -Body '{}'
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/HeartBeat?wxid=<wxid>' -Body '{}'
```

登录态导出 mock：

```powershell
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/Get62Data?wxid=<wxid>' -Body '{}'
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/GetA16Data?wxid=<wxid>' -Body '{}'
```

退出登录 mock：

```powershell
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/LogOut?wxid=<wxid>' -Body '{}'
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

`/Login/Newinit` 与 `/Login/HeartBeat` 会按 `wxid` 读取 62data/A16Data 生成的登录态，更新 `session_state`、`heartbeat_status`、`heartbeat_count` 等字段，并让 `/Login/GetCacheInfo` 可回读最近一次登录后状态。

`/Login/Get62Data` 与 `/Login/GetA16Data` 会按 `wxid` 导出对应登录态中的 mock 62/A16 数据，并记录 `last_export_kind` 与 `last_export_at`，为后续真实登录材料导入导出对拍保留接缝。

`/Login/LogOut` 会按 `wxid` 将登录态标记为 `logged_out`，写入退出样本，并使后续 `/Login/HeartBeat` 对同一 `wxid` 返回 `session_logged_out`。

查询登录态：

```powershell
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/GetCacheInfo?uuid=<uuid>' -Body '{}'
Invoke-RestMethod -Method Post 'http://127.0.0.1:7056/Login/GetCacheInfo?cache_key=<cache_key>' -Body '{}'
```

默认样本目录：

```text
.scratch/samples
```

## 当前协议封包 mock 帧

`internal/protocol` 当前提供的是 mock-first 协议帧，不是最终真实微信 `Pack` / `UnpackBusinessPacket` 协议。它的目标是先固定一个可测试、可落盘、可检测损坏数据的协议边界，后续真实协议还原时在同一模块内逐步替换。

当前帧结构：

```text
magic(4) = CBS1
version(1)
flags(1)
operation_length(2, big-endian)
payload_length(4, big-endian)
payload_crc32(4, big-endian)
operation bytes
payload bytes
```

可单独运行协议样本测试：

```powershell
go test ./internal/protocol -count=1
```

测试覆盖：

- Pack 输出稳定十六进制帧；
- Unpack 还原 `operation`、`payload`、`flags`；
- hex 输入输出往返；
- 样本 JSON 落盘，包含 `request`、`packed`、`unpacked`、`debug`；
- magic、长度和 CRC 损坏时返回稳定错误。

## 当前 Hybrid ECDH mock 接口

`internal/protocol` 还提供 `HybridECDHPackIOS`、`HybridECDHPackAndroid` 和按平台分发的 `HybridECDHPack`。这些接口当前仍是 mock-first 占位实现，不是真实微信 Hybrid ECDH 加密结果；它们复用当前 `PackBusinessPacket` 帧，以便先固定 iOS / Android 的协议接缝、摘要字段和样本格式。

当前 Hybrid 摘要包含：

- `platform`：`ios` 或 `android`；
- `pack_kind`：`hybrid_ecdh_ios_placeholder` 或 `hybrid_ecdh_android_placeholder`；
- `operation`：业务操作名；
- `payload_sha256`、`payload_length`：payload 安全摘要；
- `packed_hex`：mock 帧十六进制；
- `debug`：帧头、长度和 CRC 摘要。

登录 mock 链路中的 `/Login/GetQR`、`/Login/62data`、`/Login/A16Data` 已改为通过该模块生成 `protocol` 摘要，避免 Hybrid 占位逻辑散落在 HTTP 控制器中。

可单独运行 Hybrid 协议测试：

```powershell
go test ./internal/protocol -run Hybrid -count=1
```
