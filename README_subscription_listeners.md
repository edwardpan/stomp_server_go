# STOMP 订阅监听功能

本模块新增了订阅监听功能，允许在WebSocket服务器启动时添加多个订阅监听程序，当客户端订阅匹配的topic时，服务端会立即通过回调函数收到通知。

## 功能特性

- **模式匹配**: 支持使用 `{参数名}` 占位符定义topic模式
- **参数提取**: 自动解析topic中的动态参数并传递给回调函数
- **异步处理**: 回调函数使用异步方式调用，不会阻塞主流程
- **通用能力**: 同类型但不同动态参数的订阅会发往同一个回调函数
- **错误恢复**: 回调函数panic时会自动恢复并记录错误日志

## 使用方法

### 1. 创建STOMP服务器

```go
upgrader := websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool {
        return true
    },
}
server := NewStompServer(upgrader)
```

### 2. 添加订阅监听器

```go
// 监听任务更新订阅
err = server.AddSubscriptionListener("/topic/mission/{missionId}/updates", func(topic string, params map[string]string) {
    missionId := params["missionId"]
    fmt.Printf("任务 %s 订阅了更新: %s\n", missionId, topic)
    // 在这里实现你的业务逻辑
})
```

### 3. 启动服务器

```go
http.HandleFunc("/ws", server.HandleWebSocket)
http.ListenAndServe(":8080", nil)
```

## 模式匹配规则

- 使用 `{参数名}` 定义动态参数
- 参数名可以是任意有效的标识符
- 动态参数匹配除 `/` 之外的任意字符
- 模式必须完全匹配topic路径

### 示例模式

| 模式 | 匹配的topic | 提取的参数 |
|------|-------------|------------|
| `/topic/mission/{missionId}/updates` | `/topic/mission/MISSION001/updates` | `{"missionId": "MISSION001"}` |

## 回调函数签名

```go
type SubscriptionListenerCallback func(topic string, params map[string]string)
```

- `topic`: 客户端实际订阅的完整topic路径
- `params`: 从topic中提取的参数键值对

## 工作流程

1. 服务器启动时，通过 `AddSubscriptionListener` 添加监听器
2. 客户端通过WebSocket连接并发送SUBSCRIBE帧
3. 服务器处理订阅请求，成功后触发匹配的监听器
4. 监听器异步调用回调函数，传入topic和解析的参数
5. 回调函数执行业务逻辑（如开始数据流推送）

## 注意事项

- 回调函数在独立的goroutine中执行，需要注意并发安全
- 如果回调函数发生panic，会被自动捕获并记录错误日志
- 同一个模式可以添加多个监听器，都会被触发
- 参数提取是基于正则表达式匹配，确保模式定义正确

## 完整示例

参考 `example_usage.go` 文件查看完整的使用示例。