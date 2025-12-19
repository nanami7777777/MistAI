# MongoDB 迁移说明文档

## 概述

本项目已成功从 SQLite 数据库迁移到 MongoDB 数据库。新的数据结构采用了更适合 NoSQL 的嵌入式文档模式，提供了更好的性能和可扩展性。

## 新的数据结构设计

### 会话（Session）集合结构

```json
{
    "_id": ObjectId("..."),           // 会话ID
    "type": "bot",                    // 会话类型：固定为 "bot"（AI助手对话）
    "name": "新对话",                 // 会话名称
    "participants": ["user", "assistant"], // 参与者列表
    "created_at": ISODate("2025-01-20T10:00:00Z"),     // 会话创建时间
    "last_message_at": ISODate("2025-01-20T10:35:00Z"), // 最新消息时间
    "message_count": 42,              // 消息总数
    "is_deleted": false,              // 软删除标记
    
    // 核心：嵌入式消息数组
    "messages": [
        {
            "mid": 1,                     // 消息序号（会话内唯一）
            "sender_id": "user",          // 发送者：user 或 assistant
            "content": "你好！",          // 消息内容
            "timestamp": ISODate("2025-01-20T10:30:00Z"), // 消息时间
            "status": "sent"              // 消息状态：sent, read, deleted
        },
        {
            "mid": 2,
            "sender_id": "assistant",
            "content": "你好！很高兴为您服务。",
            "timestamp": ISODate("2025-01-20T10:31:00Z"),
            "status": "sent"
        }
    ]
}
```

## 主要改进

### 1. 数据结构优化
- **嵌入式文档**：消息直接嵌入到会话文档中，减少查询次数
- **原子性操作**：单个文档操作保证数据一致性
- **更好的性能**：一次查询获取完整对话历史

### 2. 索引优化
- `is_deleted + last_message_at`：快速获取活跃会话列表
- `name`：支持会话名称全文搜索
- `messages.content`：支持消息内容全文搜索
- `participants + type`：支持按参与者和类型筛选

### 3. 功能增强
- **消息状态管理**：支持 sent, read, deleted 状态
- **消息序号**：每个会话内消息有独立的递增序号
- **软删除**：消息和会话都支持软删除，可恢复
- **实时统计**：自动维护消息计数和最后消息时间

## MongoDB 配置

### 1. 配置文件更新
`config.json` 中新增了 MongoDB 配置：
```json
{
  "mongodb": {
    "connection_string": "mongodb://localhost:27017",
    "database_name": "chat_assistant",
    "connection_timeout": 10
  }
}
```

### 2. 环境要求
- MongoDB 4.4 或更高版本
- 默认连接地址：`mongodb://localhost:27017`
- 数据库名称：`chat_assistant`
- 集合名称：`sessions`

## 兼容性说明

### 1. API 兼容性
- 所有原有的 API 接口保持不变
- `uint` 类型的 ID 通过时间戳转换保持兼容
- 历史消息格式保持一致

### 2. 功能兼容性
- ✅ 创建/删除/重命名对话
- ✅ 发送/删除消息
- ✅ 加载历史消息
- ✅ 清空对话记录
- ✅ 对话切换

### 3. 数据迁移
由于采用了全新的数据结构，旧的 SQLite 数据无法直接迁移。如果需要保留历史数据，需要编写专门的迁移脚本。

## 使用说明

### 1. 启动 MongoDB
确保本地 MongoDB 服务正在运行：
```bash
# Windows
net start MongoDB

# macOS/Linux
sudo systemctl start mongod
```

### 2. 运行应用
```bash
go run main.go
```
或
```bash
go build -o app.exe .
./app.exe
```

### 3. 验证连接
应用启动时会自动：
- 连接到 MongoDB
- 创建必要的索引
- 初始化默认会话（如果不存在）

## 新增功能

### 1. 高级搜索
```go
// 搜索对话（按名称或消息内容）
conversations, err := SearchConversations("关键词", 10)
```

### 2. 统计信息
```go
// 获取统计数据
stats, err := GetConversationStats()
// 返回：总对话数、总消息数、当前对话ID
```

### 3. 会话详情
```go
// 获取完整会话信息（包含所有消息）
session, err := GetSessionByObjectID(objectID)
```

## 性能特点

### 1. 优势
- **读性能**：单次查询获取完整对话
- **写性能**：原子性更新，无需多表操作
- **扩展性**：支持水平扩展和分片
- **灵活性**：Schema-free，易于扩展字段

### 2. 注意事项
- **文档大小限制**：MongoDB 单文档最大 16MB
- **内存使用**：大对话会占用更多内存
- **建议**：超长对话可考虑消息归档策略

## 故障排查

### 1. 连接问题
```
错误：连接MongoDB失败
解决：检查 MongoDB 服务是否启动，连接字符串是否正确
```

### 2. 权限问题
```
错误：authentication failed
解决：检查 MongoDB 用户权限配置
```

### 3. 索引问题
```
错误：创建索引失败
解决：检查数据库权限，清空现有数据重新初始化
```

## 开发建议

### 1. 最佳实践
- 使用 ObjectID 作为主键，避免自增ID
- 合理使用索引，避免全表扫描
- 定期清理软删除的数据
- 监控文档大小，避免超出限制

### 2. 扩展方向
- 实现消息分页加载
- 添加消息搜索功能
- 实现对话导出/导入
- 添加消息统计和分析

## 版本信息

- MongoDB Driver: `go.mongodb.org/mongo-driver v1.11.3`
- Go Version: 1.24.3
- 迁移日期：2025年1月
- 兼容性：完全兼容原有 API