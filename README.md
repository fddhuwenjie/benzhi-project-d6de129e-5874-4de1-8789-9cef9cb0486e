# 化石来源治理服务

该项目为古生物野外队和馆藏接收人员提供化石标本地层来源治理。服务把发现建档、地层复核、受控采掘、封签交接、实验室验收和确定性归档串成可追溯流程，并通过版本化 HTTP JSON API 提供访问。

## 构建、运行与测试

```bash
go test ./...
go run ./cmd/fossilledger -addr=127.0.0.1:19081
go run ./cmd/fossilledger -self-check -addr=127.0.0.1:19081
```

监听地址可通过 `-addr=127.0.0.1:<port>` 指定，也可使用仅包含端口号的 `PORT` 环境变量。默认地址为 `127.0.0.1:19081`。服务使用临时内存数据，生产部署可在入口处替换持久化实现。

## API 概览

所有写入请求使用 JSON，并携带 `request_id`、`actor_id` 和 `expected_revision`；建档请求不需要 `expected_revision`。服务拒绝未知字段，同一 `request_id` 仅允许原载荷重放。

- `POST /v1/cases`：创建草稿案件。
- `PATCH /v1/cases/{id}`（也可使用 `POST .../revise`）：修订草稿现场字段。
- `POST .../review`、`POST .../review-decision`：追加复核轮次并由独立复核人决定。
- `POST .../specimens`：通过 `specimens` 数组原子登记同一采掘批次；未传封签时稳定生成封签。
- `POST .../seal-replace`、`POST .../extraction-complete`：更换未交接标本封签，并按 `batch_counts` 清点采掘名册。
- `POST .../transfers`、`POST .../custody-resolve`：登记连续交接和逐项闭合数量、封签差异。
- `POST .../intake`、`POST .../intake-resolve`：通过 `items` 数组逐件验收并记录差异处置。
- `POST .../archive`、`GET .../manifest`：校验摘要链并保存、下载确定性档案清单。
- `GET .../archive-preflight`：归档前返回结构化完整性预检报告；`GET .../batches` 返回采掘批次看板。
- `GET /v1/cases`：按 `status`、`field_lead`、`current_custodian` 和创建时间筛选并用签名游标分页，返回状态统计。
- `POST .../specimens` 可携带 `action=correction|retract` 更正或撤回未交接标本；也可使用 `specimen-correction`、`specimen-retract` 动作。
- `GET .../batches?batch=...` 返回封签批次快照；`GET .../transfers` 查询交接责任链和开放差异；`GET .../intake-report` 返回验收问题分类及补交进度。
- `GET .../audit?limit=...&cursor=...`：使用案件绑定的安全游标分页读取审计链，也支持 `actor_id`、`event_type`、`from_revision`、`to_revision` 和时间范围筛选，响应包含 `chain_valid`、匹配总数和类型统计。

`GET /v1/cases/{id}` 返回复核历史、批次清点、封签更换、当前责任人、结构化暂停原因及逐件验收结论。健康检查为 `GET /healthz`。
