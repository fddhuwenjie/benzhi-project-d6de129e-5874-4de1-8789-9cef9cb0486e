# BENZHI_README

## 项目说明
- 项目：benzhi-project-d6de129e-5874-4de1-8789-9cef9cb0486e
- 项目用途：化石来源治理服务将发现建档、地层复核、受控采掘、封签交接、馆藏验收和确定性归档串成可追溯且可冻结的案件流程。
- Go 工具链：`golang:1.22`
- 前端工具链：无

## 项目描述
- 项目名称：fossil-provenance-ledger
- 项目介绍：面向古生物野外队与馆藏接收人员的化石标本地层来源治理服务，将发现地点、地层证据、许可采掘、封签交接和实验室验收串成一条可追溯且最终冻结的案件流程，防止脱离地层背景的标本进入科研馆藏。
- 项目概述：面向古生物野外队与馆藏接收人员的化石标本地层来源治理服务，将发现地点、地层证据、许可采掘、封签交接和实验室验收串成一条可追溯且最终冻结的案件流程，防止脱离地层背景的标本进入科研馆藏。
- 核心工作流：来源案件从草稿建档开始，提交地层证据后进入来源复核；复核通过才可授权采掘，完成采掘后按封签逐段移交，馆藏实验室核验来源、数量与封签，异常则进入暂停并完成处置，全部一致后生成确定性档案包并冻结为不可变已归档状态。
- 对外接口：Go 服务提供版本化 HTTP JSON API，野外队和馆藏人员通过案件、复核、采掘、交接、验收及归档端点推进唯一流程；服务支持 -addr=127.0.0.1:<port> 与 PORT 环境变量，默认监听 127.0.0.1:19081，且不默认绑定 0.0.0.0。

## 标准构建、运行和测试命令
进入容器后执行：
cd '/app' && GOTOOLCHAIN=local go build ./...

cd /app && GOTOOLCHAIN=local go run ./cmd/fossilledger -self-check -addr=127.0.0.1:19081

cd '/app' && GOTOOLCHAIN=local go test ./...

## Docker 构建和进入容器
chmod +x build_benzhi_docker.sh

./build_benzhi_docker.sh benzhi-project-d6de129e-5874-4de1-8789-9cef9cb0486e-amd64 linux/amd64

./build_benzhi_docker.sh benzhi-project-d6de129e-5874-4de1-8789-9cef9cb0486e-arm64 linux/arm64

docker run -it benzhi-project-d6de129e-5874-4de1-8789-9cef9cb0486e-amd64:latest

## 题目验证命令
1. 预期退出码 0：`go test ./...`
2. 预期退出码 0：`go run ./cmd/fossilledger -self-check -addr=127.0.0.1:19081`
