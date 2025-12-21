# MistAI 桌面 AI 助手

目前存在llm回复过慢的问题，需要测试其他模型(当前项目中所用api是网上的公开免费api)

MistAI 是一个基于 Go 与 Fyne 的本地桌面 AI 助手应用，支持调用兼容 OpenAI 的大语言模型接口（如 OpenAI、智谱 GLM、自定义 OpenAI 风格 API），并通过 MongoDB 持久化对话、词典与翻译记录。

应用提供多会话管理、词典/翻译历史面板、全局快捷键、流式输出与可视化配置界面，适合作为日常问答、翻译与单词管理工具。

目前只支持windows平台
## 功能特性

- 多会话对话
  - 左侧会话列表：新建、切换会话
  - 会话消息保存在 MongoDB 中，可持久化历史记录
- 流式响应与控制
  - 模型回复以流式方式展示
  - 支持「停止输出」按钮中断当前生成
- 智能翻译与词典模式
  - 将翻译结果与词典结构化数据写入数据库
  - 右侧「历史记录」列表展示查询过的单词
- 历史记录与详情
  - 悬停查看单词详细解释
  - 支持从历史中快速回顾或复制内容
- 全局快捷键与剪贴板
  - 在系统中按下 `Ctrl + C` 时，应用会读取当前剪贴板内容并填入输入框（`gui/gui.go:125`）
  - 自动聚焦到输入框，方便一键提问
- 输入体验优化
  - 多行输入框（`MyWidget/myWidget.go:68`）
  - `Enter` 发送消息，`Shift + Enter` 换行
- 可视化配置
  - 内置设置窗口：选择 API 类型、填写 API Key、API URL 和模型名称（`gui/gui.go:801`）
  - 配置信息保存在根目录的 `config.json` 中（`config/config.go:7`）

## 技术栈

- 语言与运行时
  - Go（`go.mod` 中声明为 `go 1.24.3`，建议使用最新稳定版 Go）
- 桌面 UI
  - `fyne.io/fyne/v2`：跨平台桌面 UI 框架（`gui/gui.go:1`）
- 数据库
  - MongoDB 作为持久化存储（`database/db.go`、`database/message.go:1`）
  - 使用官方驱动 `go.mongodb.org/mongo-driver`
- AI/LLM 调用
  - 自定义 `llm` 包封装聊天接口（`llm/llm.go:1`）
  - 支持 OpenAI/智谱 GLM/自定义兼容 OpenAI 协议的 API
- 其他
  - `github.com/robotn/gohook`：全局快捷键监听（`gui/gui.go:125`）
  - `github.com/atotto/clipboard`：系统剪贴板访问（`gui/gui.go:127`）
  - `github.com/sirupsen/logrus`：结构化日志（`main.go:19`）

## 目录结构

仅列出主要目录与文件，帮助快速理解项目结构：

- `main.go`：应用入口，负责加载配置、初始化数据库并启动 GUI（`main.go:11`）
- `config/`
  - `config.go`：定义 `AppConfig`、`MongoConfig` 并负责读写 `config.json`（`config/config.go:7`）
- `database/`
  - `db.go`：MongoDB 连接、初始化与关闭逻辑
  - `message.go`：词典与翻译等业务数据结构与数据库操作（`database/message.go:1`）
- `service/`
  - `message_service.go`：对话消息服务，负责与 LLM 通信与流式输出（`service/message_service.go:1`）
  - `conversation_service.go`：会话管理相关逻辑
  - `config_service.go`：包装配置读写逻辑，供 GUI 使用（`service/config_service.go:1`）
- `gui/`
  - `gui.go`：Fyne 界面构建、会话列表、聊天区域、历史面板以及设置窗口（`gui/gui.go:1`）
- `llm/`
  - `llm.go`：封装聊天接口调用逻辑，构造系统提示、控制上下文长度等（`llm/llm.go:1`）
- `MyWidget/`
  - `myWidget.go`：自定义组件，如可悬停标签与支持 `Shift + Enter` 换行的多行输入框（`MyWidget/myWidget.go:68`）
- `example/`
  - `example.go`：Fyne 性能测试示例（`example/example.go:1`）

## 环境要求

- Go：建议 Go 1.21+（`go.mod` 当前为 `1.24.3`）
- MongoDB：建议 5.0 或更高版本
- 操作系统：支持 Fyne 的主流桌面系统（Windows/macOS/Linux）

## 配置说明

应用运行时会从项目根目录读取 `config.json`，对应结构定义在 `config/config.go:7` 中：

```json
{
  "provider": "openai",
  "api_key": "YOUR_API_KEY",
  "api_url": "https://api.openai.com/v1/chat/completions",
  "model": "gpt-4.1-mini",
  "mongodb": {
    "connection_string": "mongodb://localhost:27017",
    "database_name": "chat_assistant",
    "connection_timeout": 10
  }
}
```

字段说明：

- `provider`：API 类型
  - `"zhipu_glm"`：智谱 GLM（默认 URL 与模型已在 GUI 中预设，`gui/gui.go:27`）
  - `"openai"`：OpenAI 官方接口
  - `"custom"`：自定义兼容 OpenAI 协议的第三方接口（如代理服务）
- `api_key`：你的模型服务 API Key（请不要将真实 Key 提交到版本库）
- `api_url`：聊天接口地址，例如 OpenAI 或代理地址
- `model`：使用的模型名称
- `mongodb.connection_string`：MongoDB 连接字符串
- `mongodb.database_name`：数据库名称
- `mongodb.connection_timeout`：连接超时时间（秒）

> 安全提示：仓库中的示例 `config.json` 仅用于本地开发，请务必替换为你自己的 Key，避免泄露到公共仓库。

## 快速开始

1. 安装依赖
   - 安装 Go（1.21+）
   - 安装并启动 MongoDB
2. 获取代码
   - 如果是从远程仓库拉取：
     ```bash
     git clone <your-repo-url> MistAI
     cd MistAI
     ```
   - 如果你已经在本地有此项目，直接进入项目根目录即可。
3. 配置 `config.json`
   - 在项目根目录创建或修改 `config.json`，填入你的 API 配置与 MongoDB 信息（参见上文示例）
4. 启动 MongoDB
   - Windows：
     ```bash
     net start MongoDB
     ```
5. 运行应用
   - 调试运行：
     ```bash
     go run main.go
     ```
   - 或构建二进制：
     ```bash
     go build -o app.exe .
     ./app.exe
     ```

启动后会看到标题为「AI 助手」的桌面窗口（`gui/gui.go:159`）。

## 使用说明

- 基本对话
  - 在底部输入框中输入问题，按 `Enter` 发送
  - 使用 `Shift + Enter` 换行，不立即发送（`MyWidget/myWidget.go:80`）
- 会话管理
  - 左侧面板用于创建和切换会话
  - 不同会话的消息会分别保存到 MongoDB 中
- 历史记录与词典
  - 右侧「历史记录」列表显示查询过的单词或翻译条目（`gui/gui.go:175`）
  - 悬停可查看详细释义与例句
- 设置与模型
  - 点击界面中的「⚙」按钮打开设置窗口（`gui/gui.go:801`）
  - 可切换 API 类型（OpenAI、智谱 GLM、自定义）、修改 URL、模型与 API Key
- 全局快捷键
  - 在系统中复制文本后按 `Ctrl + C`，应用会自动将剪贴板内容填入输入框并聚焦（`gui/gui.go:125`）

## 开发与调试

- 日志
  - 应用在启动时配置 `logrus` 日志，并将日志输出到 `app.log`（`main.go:23`）
  - 便于排查连接 MongoDB 或调用 LLM 过程中出现的问题
- 数据库结构
  - 词典与翻译相关的结构定义在 `database/message.go` 中（`database/message.go:18` 等）
  - MongoDB 迁移与设计说明见 `MONGODB_MIGRATION.md`
- 实验代码
  - `example/example.go` 提供了一个 Fyne 性能测试示例，可独立运行（`example/example.go:10`）

## 常见问题

- 无法连接 MongoDB
  - 检查 MongoDB 服务是否已启动
  - 检查 `config.json` 中的 `connection_string` 是否正确
- 模型调用失败
  - 检查 `api_key`、`api_url` 与 `model` 是否正确
  - 确认当前网络环境或代理设置支持访问对应的模型服务
- 界面无响应或闪退
  - 确保 Go 和依赖库版本与 `go.mod` 兼容
  - 可删除本地构建缓存后重新运行：`go clean -modcache`

如果你希望继续扩展 MistAI（例如增加本地模型、更多快捷键或导出功能），可以从 `gui/`、`service/` 与 `llm/` 目录入手进行二次开发。
