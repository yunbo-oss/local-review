# 简历写法

以下数字来自 2026-08-19 的空环境容器回归和真实模型评测；旧运行时的 `96.43% vs 46.43%` 对照不建议写成当前架构成绩。

## 推荐三条版

```latex
\section{项目经历}
\row{\textbf{个人开发}\enspace \href{https://github.com/yunbo-oss/local-review}{GitHub}}{\textbf{2025.07 - 2026.07}}
\module{本地生活服务平台与智能推荐 Agent}{Go、Gin、PostgreSQL、pgvector、Redis、RocketMQ}
\begin{itemize}
  \item 设计推荐 Agent 运行框架（Harness）作为线上请求与离线评测的统一执行边界，统一上下文裁剪、运行时选择、工具注册、并发与依赖调度、调用预算、超时取消、检查点恢复和结果校验；以 Trace ID 串联 SSE 事件、OpenTelemetry 链路及 PostgreSQL 运行/工具审计，支持多实例故障定位与执行回放。
  \item 构建“大模型请求理解与查询改写—三路置信度路由—有界并行 ReAct—证据缺口续查—逐条事实校验”的推荐链路；通过候选注册屏障限制详情/评价工具只能访问检索结果，候选确定后按依赖图并行执行，证据不足时翻页或改写重搜，无事实支持时安全回退。
  \item 将业务数据、全文索引和向量统一迁移至 PostgreSQL + pgvector，使用 GIN/HNSW 索引与双路融合检索，并以来源版本保证消息幂等更新；Redis 仅承载缓存、会话与运行检查点。基于 200 家店、1000 条评价的 56 次真实模型执行，任务成功率 \textbf{92.86\%}、成功回答有据率 \textbf{100\%}、基础设施错误率 \textbf{0\%}；60 题检索任务全部成功，P95 为 \textbf{17ms}。
\end{itemize}
```

## 空间紧张的两条版

```latex
\section{项目经历}
\row{\textbf{个人开发}\enspace \href{https://github.com/yunbo-oss/local-review}{GitHub}}{\textbf{2025.07 - 2026.07}}
\module{本地生活服务平台与智能推荐 Agent}{Go、Gin、PostgreSQL、pgvector、Redis、RocketMQ}
\begin{itemize}
  \item 设计推荐 Agent 运行框架（Harness），统一请求理解与改写、三路置信度路由、有界并行工具调用、预算/超时/恢复及逐条事实校验；以候选注册屏障约束详情和评价工具只能访问检索结果，并用 Trace ID 串联 SSE、分布式链路与 PostgreSQL 运行审计。
  \item 将业务数据、全文索引和向量统一持久化至 PostgreSQL + pgvector，使用 GIN/HNSW 与双路融合检索、来源版本幂等更新，Redis 仅承载可过期状态；在 200 家店、1000 条评价的真实模型评测中取得 \textbf{92.86\%} 任务成功率、\textbf{100\%} 成功回答有据率，60 题检索任务全部成功且 P95 为 \textbf{17ms}。
\end{itemize}
```

面试时不要把“记忆 Agent”作为单独模块介绍。更准确的说法是：长期画像在应用层按版本读取，Harness 只注入本轮相关的结构化偏好；回答校验成功后才允许更新画像。Redis 中的运行检查点服务于恢复，也不是长期记忆。
