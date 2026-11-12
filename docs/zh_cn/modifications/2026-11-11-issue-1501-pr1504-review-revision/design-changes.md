# BFE PR #1504 评审修订方案：#1501 修复的 nil 防御平移问题与契约固化修正

- 来源：[PR #1504 review（xuleiming，CHANGES_REQUESTED）](https://github.com/bfenetworks/bfe/pull/1504#pullrequestreview-5856349973)——针对 #1501 修复（`698ff57a`）的评审意见。
- 背景文档：`2026-11-11-issue-1501-nonstream-truncated-body-nil-panic-fix/design-changes.md`（本修订落地后需同步更新其中不一致处，见 §五）。
- 代码库：`bfe/`（行号基于当前 HEAD `698ff57a`，评审各点已逐条源码复核）。**不需要产品裁定**（全部为实现正确性问题）。

## 一、评审意见逐条裁定（全部接受）

| # | 评审意见 | 复核结论 | 处置 |
| --- | --- | --- | --- |
| 1 | `DoResponseProcess` 的 `res.Body == nil` 早退把 panic 平移到下游（调用方丢弃返回值，`copyResponse`/`res.Body.Close()` 仍 nil panic）；且**去掉早退、仅保留构造期守卫反而是正确路径**——`CreateEventDecoder` 会把工厂错误写入 `bp.err`，`res.Body = bp` 非 nil，转发返回读错误 ⇒ `BK_READ_RESP_BODY` | **属实，双重确认**：`mod_body_process/mod_body_process.go:181` 以语句形式调用、确实丢弃返回值；`body_process.go:129-138` `CreateEventDecoder` 已有 `bp.err` 降级机制；早退位于 `ResProcess.Inc(1)` 之后还会虚增计数 | **删除早退**（§二.1） |
| 2 | `tokenReadResponseHandler` 缺 `bodyAccessor != nil` 判断——`GetBodyAccessor()` 在 `r.Body == nil` 时返回 `(nil, nil)`，随后 `bodyAccessor.GetBytes()` nil 接口 panic | **属实**：`bfe_http/response.go` 开头即 `if r.Body == nil { return nil, nil }`，当前代码只判 `err` | **补 nil 守卫**（§二.2） |
| 3 | `TestDoResponseProcessNilBody` 断言 `bp == nil` 固化了错误契约；真正不变量是"返回后 `res.Body` 对下游 Read/Close 安全" | **属实**：与 §二.1 同根 | **测试改写**（§三.1） |
| 4 | `NewContentTypeDecoder` 三个分支均以 `dec, _` 丢弃工厂错误，`ctd.dec` 可能为 nil 后 `ctd.dec.Decode()` panic——删除早退后必须同步修 | **属实**：`body_process.go` `NewContentTypeDecoder` 全部三分支吞错；且 `NewSSEEventDecoder(nil)`（`llm_util.go:250`）同样存 nil reader 延迟 panic，与 JsonDecoder 同族 | **错误传播 + 三分支统一守卫**（§二.3） |
| 5 | 设计文档声称的全链定点单测未实现 | 属实 | 补链式用例（§三.2） |
| 6 | 集成缺 `EstimateToken=false` 臂（`unrecognized` + `ai_zero_cost_reason=upstream_dropped` + 零扣费） | 属实 | TC-15 增子臂（§三.3） |
| 7 | TC-15 未断言 `PanicClientConnServe`；且 BFE 仍在运行时读 `exception.log`，日志缓冲可致"无 panic"断言空转 | 属实 | 先停进程后读日志 + 指标断言（§三.4） |
| 8 | 文档/实现不一致：`return nil, err` vs `errors.New("can't get body")`（`io.ErrUnexpectedEOF` 被丢弃）；`GetBodyAccessor` 注释"deliver the remaining bytes"与事实不符（前缀已被 `io.ReadAll` 消费，残余为 0，转发层拿到的是读错误） | 属实 | 错误包装保留 `%w` + 注释改写（§二.4、§五） |

评审"确认正确的部分"（`response.go` 先取后赋、`MarkResponseCompleted` 内移、截断时 `CompletionTokens=-1` 使 `CalcReqUsedQuota` 返 0 只扣 prompt 播种、TC-15 计费算术与 TC-07 口径一致）——**维持不变**。

## 二、代码修订

### 1. 删除 `DoResponseProcess` 的 nil 早退（mod_body_process）

删除 `if res.Body == nil { return nil }` 块（含注释）。nil body 的降级改由构造期守卫驱动：

```
NewBodyProcessor(res.Body /* nil */)
  → CreateEventDecoder(NewJsonDecoder) → NewJsonDecoder(nil) 返回 error
  → bp.err = "create event decoder: json decoder: nil body source"
  → bp 非 nil，mod_body_process.go:181 调用点将 res.Body 替换为 bp
  → 转发时 bp.Read 返回 bp.err → copyResponse 错误 → BK_READ_RESP_BODY（#1474 路径）
```

这一路径同时消除评审指出的 `ResProcess` 虚增问题（不再有任何早退）。

### 2. `tokenReadResponseHandler` 补 bodyAccessor nil 守卫（mod_ai_token_auth）

```go
		if bodyAccessor, err := res.GetBodyAccessor(); err == nil && bodyAccessor != nil {
```

（`GetBodyAccessor` 在 `r.Body == nil` 时返回 `(nil, nil)`，单测覆盖该形态。）

### 3. `NewContentTypeDecoder` 错误传播 + 三分支统一 nil 守卫

```go
func NewContentTypeDecoder(source io.Reader, contentType string) (EventDecoder, error) {
	newDec := func(fac func(io.Reader) (EventDecoder, error)) (EventDecoder, error) {
		if source == nil {
			return nil, fmt.Errorf("content-type decoder: nil body source")
		}
		return fac(source)
	}
	var (
		dec EventDecoder
		err error
	)
	switch mediaTypeOnly(contentType) {
	case "application/sse", "text/event-stream", "application/x-sse":
		dec, err = newDec(NewSSEEventDecoder)
	case "application/json", "application/ndjson", "application/x-ndjson":
		dec, err = newDec(NewJsonDecoder)
	default:
		dec, err = newDec(NewLineDecoder)
	}
	if err != nil {
		return nil, err   // 不再吞错，杜绝 ctd.dec == nil 的延迟 panic
	}
	return &ContentTypeDecoder{contentType: contentType, dec: dec}, nil
}
```

`NewSSEEventDecoder` 自身也补 nil 守卫（`llm_util.go:250`，与 Json/Line 同族同因）。检查 `ContentTypeDecoder` 的全部调用点（`body_process.go:307/387/467`）对 error 的处理——工厂错误进入 `CreateEventDecoder` 的 `bp.err` 路径，无需额外改动。

### 4. `GetBodyAccessor` 错误包装与注释（bfe_http）

```go
		newBody, err := NewBytesBody(r.Body, GetAccessibleBodySize())
		if err != nil {
			// issue #1501: never nil out r.Body here. io.ReadAll may have
			// consumed a prefix before the truncation error; the retained
			// stream only yields that read error, and the forwarding path
			// terminates the response as truncated (issue #1474 yardstick).
			return nil, fmt.Errorf("can't get body: %w", err)
		}
```

`%w` 保留 `io.ErrUnexpectedEOF` 供调用方/errors.Is 判定（评审第 8 点）。

## 三、测试修订

### 1. 改写 `TestDoResponseProcessNilBody`（锁定正确契约）

不再断言 `bp == nil`；断言降级不变量：

```go
	res := &bfe_http.Response{StatusCode: 200, Body: nil}
	bp := m.DoResponseProcess(req, res, &BodyProcessConfig{Dec: "json"})
	if bp == nil {
		t.Fatal("a nil body must degrade to a non-nil processor carrying the error")
	}
	res.Body = bp                       // 调用点语义（mod_body_process.go:181）
	if _, err := io.ReadAll(res.Body); err == nil {
		t.Fatal("reading the degraded processor must surface the decoder error")
	}
	if err := res.Body.Close(); err != nil {
		t.Fatalf("close must be safe for the forwarding path: %v", err)
	}
```

### 2. 补全链定点单测（评审第 5 点）

`mod_ai_token_auth` 与 `mod_body_process` 联合：截断 body 响应（复用 `truncatedBodyReadCloser`）依次经过 `tokenReadResponseHandler` → `DoResponseProcess` → `Read`，断言：不 panic；`IsResponseCompleted()==false`；`res.Body` Read/Close 安全且返回错误；最终错误可经 `recordCopyError` 归因 `BK_READ_RESP_BODY`（以错误值透传断言，不起 BFE 进程）。

### 3. TC-15 增 `EstimateToken=false` 子臂（评审第 6 点）

仿 TC-06 的 `newTestEnvWithEstimateToken(t, false)`：`upstreamDropped && !estimateBillable` ⇒ 余额不变、field 764=`unrecognized`、field 765=`upstream_dropped`。

### 4. TC-15 断言加固（评审第 7 点）

- **先停进程、后读日志**：将"exception.log 无 panic"断言移到 `stopAndParseLogs()` 之后（进程停止冲刷日志，断言不再空转）；access log 解析与异常日志读取同序进行。
- **`PanicClientConnServe` 不增**：SC29 `testEnv` 目前丢弃 monitor 端口（`StartBFE` 第二返回值）——捕获后从 `/monitor/state`（或既有 server state 端点，落码时核实路径）读取该计数器前后差分断言为 0。
- 客户端截断断言、计费精确断言、`err_code`/`ai_usage_source`/`ai_stream` 断言维持。

## 四、回归与验收

- 三包单测（`bfe_http` / `mod_body_process` / `mod_ai_token_auth`）全量 + 新增/改写用例；
- SC29 全场景（TC-01~TC-15）回归；
- 评审原验收判据不变：同形态请求无 panic、RequestLog `BK_READ_RESP_BODY`、`upstream_dropped` 计费口径、客户端截断、流式不回归（issue #1501 验收 1-5）。

## 附：实现附记（与评审建议的字面偏差一处）

评审建议的备选防御"`res.Body = bfe_http.EofReader`（或 `NewBodyProcessor` 归一化）"在实现时被验证**不可取**：归一化后解码器工厂收到的是非 nil reader，构造成功，`bp.err` 不设置——`Read` 走正常 EOF 路径，降级信号丢失（nil body 被静默当作空 body 转发完成，而非返回读错误归因 `BK_READ_RESP_BODY`）。落地改为：`NewBodyProcessor` 保留 nil source，四个解码器工厂 nil 守卫 + `CreateEventDecoder` 记 `bp.err`（`Read` 短路返回错误），`BodyProcessor.Close` 补 nil-source 守卫（`Close` 安全）。单测锁定的正是这组不变量。

## 第二轮评审修订（2026-11-12，xuleiming：结论"核心修复已闭环，TC-15'不 panic'断言未生效，需小修后合入"）

评审确认上轮 6 项全部闭环（逐条复述无异议），新阻塞项与建议均经源码复核**全部属实**：

### 阻塞项：`proxyStateCounter` 读错字段，"不 panic"断言空转

- **事实**：`/monitor/proxy_state` 的 body 是 `metrics.MetricsData` 直序（go-lib `web-monitor/metrics/metrics.go:343-348`：`Prefix/Kind/GaugeData/CounterData/StateData`），**没有 `SCounters`**（那是 `module_state2.StateData` 的字段）；计数器键经 `convert()`（同文件 :307 `unicode.ToUpper`）从 CamelCase 转 UPPER_SNAKE：`PanicClientConnServe → PANIC_CLIENT_CONN_SERVE`。当前实现按 `SCounters` 反序列化，`json.Unmarshal` 对未知字段不报错 ⇒ nil map ⇒ 前后恒 0 ⇒ 差分断言永不成立——正是上轮要求补的"不 panic"验证实际没有生效。
- **修复**（评审给出的形态，直接采纳）：

```go
	var data struct {
		CounterData map[string]int64 `json:"CounterData"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse proxy_state failed: %v, body: %s", err, raw)
	}
	v, ok := data.CounterData[key]
	if !ok {
		t.Fatalf("counter %q not found in proxy_state: %s", key, raw)
	}
	return v
```

调用处 key 改为 `"PANIC_CLIENT_CONN_SERVE"`；`ok` 判定保证键名再变时**立即失败**而非静默通过。

### 非阻塞建议（全部采纳）

1. **`exception.log` 断言防路径错写**：当前 `if err == nil && len(data) > 0` 在文件不存在时静默通过。实现精化（比评审建议多走一步的实证修正）：`exception.log` 由 BFE **惰性创建**（首次异常才写文件），"无 panic"时文件本就不存在——因此改为断言**日志目录存在**（access log 刚从中解析成功，目录断言不可能空转通过），文件缺失 = 通过、非空 = 失败、其他读错误 = 失败；
2. **`reqLogs` 判空**：`reqLog := reqLogs[len(reqLogs)-1]` 前加 `if len(reqLogs) == 0 { t.Fatal(...) }`，日志缺失给明确失败信息而非 index out of range；
3. **删除重复的 `res.Body = bp`**：`DoResponseProcess` 内部已赋值（`body_process.go:403`），`TestDoResponseProcessNilBody` 与全链测试中的显式赋值冗余，删除后断言直接读 `res.Body`（更贴近调用点真实语义）。

### 验证

- TC-15 重跑：键存在性由 `ok` 判定保证——反向校验可用性：临时把 key 写错，测试必须在 `not found` 处失败（确认断言链路不再空转）；
- 三包单测 + SC29 全场景回归；
- 修复推送后回复评审请其复核 `PANIC_CLIENT_CONN_SERVE` 断言实际读数（评审可用同形态 run 交叉验证）。

## 五、背景文档同步修订（`2026-11-11-issue-1501-.../design-changes.md`）

1. §二"步骤 2"由"`DoResponseProcess` 对 nil body 返回 nil"改为"**删除早退**：nil body 降级由构造期守卫 + `bp.err` 驱动，`res.Body=bp` 非 nil 且 Read 返回错误——转发安全且归因正确"；删除"`ResProcess`"相关表述的隐含早退语义；
2. §一.1.2 补充"`GetBodyAccessor` 在 `r.Body == nil` 时返回 `(nil, nil)`"与 bodyAccessor 守卫；
3. §二.1 的 `return nil, err` 补注实现形态（`fmt.Errorf("can't get body: %w", err)`），"保留残余流"表述修正为"保留原流（残余前缀已被 io.ReadAll 消费，转发以读错误终止）"；
4. §二步骤 4 测试清单替换为本文 §三 的四个用例/子臂/加固项；
5. 新增"评审修订"一节指向本文档。
