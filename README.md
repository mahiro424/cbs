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
- 提供 `internal/protocol` mock-first Pack / Unpack、Hybrid ECDH iOS / Android、AES-GCM 解包和二进制调试入口，用于后续真实协议对拍替换。
- 提供 `internal/network` mock/real 网络层接缝；默认 mock 不访问真实服务端，real 当前返回稳定未就绪错误。
- 提供 `internal/login` 登录业务层接缝；`/Login/GetQR` 已下沉为业务层 tracer bullet。

## 运行

Windows / PowerShell 下先进入 UTF-8 会话：

```powershell
. .\scripts\enter-utf8.ps1
```

```powershell
go test ./...
go run ./cmd/server -config conf/app.conf
```

登录态存储默认使用进程内 `memory`，不依赖本机 Redis 即可跑通 mock-first 链路。需要验证 Redis 登录态接缝时，在 `conf/app.conf` 中显式设置：

```text
loginstatestore = redis
redislink = 127.0.0.1:6379
redispass = ""
redisdbnum = 7
```

`/healthz` 会输出 `login_state_store.mode`，用于确认当前运行时使用的是 `memory` 还是 `redis`。Redis 模式下，登录态会写入 `login:state:<uuid>`，并维护 `cache_key` 与 `wxid` 索引；Redis 不可用时，相关登录态保存/读取接口会返回统一 `login_state_store_error`。

网络层默认使用 mock 模式，不访问真实微信服务端：

```text
networkmode = mock
```

需要验证真实网络模式的错误接缝时，可显式设置：

```text
networkmode = real
```

当前 `real` 模式尚未接入真实 MMTLS / HTTP 发送，会返回统一 `network_error`，用于先固定配置入口、错误结构和后续替换边界。`/healthz` 会输出 `network.mode`，用于确认当前网络模式。

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

`/Login/GetQR` 当前已经由 HTTP 控制器下沉到 `internal/login` 业务层，会经过一条可验证的二维码 mock 登录链路：

1. 解析设备请求。
2. 业务层构建登录上下文和默认设备字段。
3. 生成 Hybrid ECDH iOS 协议占位摘要。
4. 通过 `internal/network` 执行 mock 网络发送，生成可观测网络阶段摘要。
5. 保存到配置选择的登录态存储（默认 `memory`，可切换到 `redis`）。
6. 将请求、协议占位、网络摘要、mock 响应和登录态摘要落盘为 JSON 样本。
7. 返回 `uuid`、`cache_key`、`protocol`、`network`、`login_state` 和 `sample_path`。

`/Login/CheckQR` 会读取 `/Login/GetQR` 生成的 `uuid` 登录态，在 mock 模式下返回稳定的 `waiting_scan` 状态，写入最近一次检查样本，并让 `/Login/GetCacheInfo` 继续读回同一登录态。

`/Login/62data` 与 `/Login/A16Data` 会分别生成 iOS / Android 的协议占位摘要、mock 登录响应、登录态和样本文件，供后续真实协议与样本对拍替换。

> 当前只有 `/Login/GetQR` 完成了 `internal/login` 业务层下沉；`/Login/62data` 与 `/Login/A16Data` 仍保留在 HTTP 控制器内，后续切片会按同一模式迁移。

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

## 当前登录态存储边界

`internal/storage` 提供 `LoginState`、`EncodeLoginState`、`DecodeLoginState`、`LoginStateStore`、`MemoryLoginStateStore` 和 `RedisLoginStateStore`。当前登录 mock 链路已经通过 `LoginStateStore` 边界保存、读取和更新登录态，不再在 HTTP 控制器内维护私有登录态结构。

当前 storage 边界覆盖：

- 登录态 JSON 序列化与反序列化；
- `uuid` 主键读取；
- `cache_key` 索引读取；
- `wxid` 索引读取；
- Redis key 规划与 Redis backend 独立读写：
  - `login:state:<uuid>`
  - `login:index:cache:<cache_key>`
  - `login:index:wxid:<wxid>`
- Redis 登录态 backend 支持可选 `AUTH`、`SELECT <db>`、`SET` 主状态、`SET` cache/wxid 索引，以及 `GET` 主状态或索引后再回读主状态；
- Redis 不可连接会返回可用 `errors.Is(err, storage.ErrRedisUnavailable)` 判断的稳定错误，Redis 返回 `-ERR` / `-NOAUTH` 会返回可用 `errors.Is(err, storage.ErrRedisCommandFailed)` 判断的稳定错误；
- Redis backend 测试使用进程内 fake Redis 夹具，不依赖真实 Redis 实例。

当前 HTTP 默认仍使用 `MemoryLoginStateStore`，但可通过 `loginstatestore = redis` 切换为 `RedisLoginStateStore`。Redis 模式已覆盖跨 Server 实例读写测试：一个 Server 写入 `/Login/62data` 登录态后，另一个 Server 可以通过 `/Login/Newinit` 和 `/Login/GetCacheInfo` 读回并更新同一 Redis 登录态。

## 当前网络层接缝

`internal/network` 提供 `Client`、`Request`、`Response` 和配置化 `mock` / `real` 模式：

- `mock`：默认模式，不访问真实服务端；返回 `mode`、`operation`、`login_kind`、`platform`、`stage`、`payload_sha256`、`payload_length` 等摘要，用于本地链路验证和样本落盘。
- `real`：当前只固定入口和错误契约，返回可用 `errors.Is(err, network.ErrRealNetworkNotReady)` 判断的稳定错误；后续真实 MMTLS / HTTP 发送会在该模式下替换实现。
- 非法网络模式会返回可用 `errors.Is(err, network.ErrNetworkConfig)` 判断的稳定配置错误。

当前 `/Login/GetQR`、`/Login/62data` 和 `/Login/A16Data` 已通过该网络接缝产生 `network` 摘要，并写入响应与样本文件。

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

## 当前 AES-GCM 解包接口

`internal/protocol` 提供 `AESGCMUnpack`、`AESGCMUnpackHex` 和语义别名 `UnpackBusinessPacketWithAESGCM`。该接口复用 `internal/algorithm.AESGCMDecrypt`，用于先固定协议层响应解包接缝；真实微信响应的业务结构、protobuf 字段或后续登录态写入规则仍等待真实样本继续补齐。

当前解包结果包含：

- `operation`：调用方标记的业务操作；
- `plaintext_hex`、`plaintext_sha256`、`plaintext_length`：明文摘要；
- `ciphertext_sha256`：密文摘要；
- `debug`：key、nonce、aad、ciphertext、plaintext 的长度信息。

十六进制样本可通过 `AESGCMUnpackHex` 直接解包；`WriteAESGCMUnpackSample` 会保存 `request`、`decrypted`、`debug` 三段 JSON，便于后续与真实响应包对拍。

可单独运行 AES-GCM 协议测试：

```powershell
go test ./internal/protocol -run AESGCM -count=1
```

## 协议二进制调试工具

`cmd/protocol-debug` 是当前 mock-first 协议帧的本地调试入口，不需要启动 HTTP 服务，也不会访问真实网络。它用于快速 inspect 十六进制封包样本，并对 expected / actual 两个样本做字节级 compare。

Inspect 示例：

```powershell
go run .\cmd\protocol-debug inspect --hex 434253310103000b00000013c463dfb64d73672e53656e6454787468656c6c6f206d6f636b2070726f746f636f6c
```

输出包含：

- `status`、`hex`、`length`；
- `packet.operation`、`packet.payload_utf8`、`packet.payload_hex`、`packet.flags`；
- `debug.magic`、`debug.version`、长度字段和 `crc32_hex`。

Compare 示例：

```powershell
go run .\cmd\protocol-debug compare --expected <expected-hex> --actual <actual-hex>
```

输出包含：

- `equal`：两个样本是否字节完全一致；
- `first_difference`：首个差异字节的偏移和值；
- `expected` / `actual`：双方各自的 inspect 摘要；
- `status=skipped` 和 `skip_reason`：expected 或 actual 样本缺失时的明确跳过原因。

可单独运行调试工具测试：

```powershell
go test .\internal\protocol .\internal\protocoldebug -count=1
```
