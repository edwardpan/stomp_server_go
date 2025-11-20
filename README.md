# STOMP WebSocket Server for Go

这是一个基于WebSocket的STOMP（Simple Text Oriented Messaging Protocol）服务器实现，兼容STOMP 1.0、1.1和1.2版本规范。

## 特性

- ✅ **多版本支持**: 兼容STOMP 1.0、1.1、1.2规范
- ✅ **WebSocket集成**: 基于gorilla/websocket实现
- ✅ **消息路由**: 支持自定义消息处理器
- ✅ **订阅管理**: 完整的订阅/取消订阅功能
- ✅ **事务支持**: 支持BEGIN/COMMIT/ABORT事务
- ✅ **心跳机制**: STOMP 1.1+版本的心跳支持
- ✅ **错误处理**: 完善的错误处理和日志记录
- ✅ **线程安全**: 并发安全的连接和订阅管理

## 快速开始

### 1. 创建STOMP服务器

```go
package main

import (
    "github.com/gogf/gf/v2/frame/g"
    "github.com/gogf/gf/v2/net/ghttp"
    "your-project/utility/stomp"
)

func main() {
    // 创建STOMP服务器
    stompServer := stomp.NewStompServer()
    
    // 设置消息处理器
    stompServer.SetMessageHandler("/topic/chat", func(conn *stomp.Connection, frame *stomp.Frame) error {
        // 处理消息逻辑
        return stompServer.SendToDestination("/topic/chat", frame.Body, nil)
    })
    
    // 创建GoFrame服务器
    s := g.Server()
    
    // 绑定WebSocket端点
    s.BindHandler("/stomp", func(r *ghttp.Request) {
        stompServer.HandleWebSocket(r.Response.Writer, r.Request)
    })
    
    s.SetPort(8080)
    s.Run()
}
```

## API 参考

### 服务器端

#### StompServer

```go
// 创建新的STOMP服务器
func NewStompServer() *StompServer

// 设置消息处理器
func (s *StompServer) SetMessageHandler(destination string, handler MessageHandler)

// 处理WebSocket连接
func (s *StompServer) HandleWebSocket(w http.ResponseWriter, r *http.Request)

// 向目标发送消息
func (s *StompServer) SendToDestination(destination string, body []byte, headers map[string]string) error
```

## STOMP协议支持

### 支持的命令

- `CONNECTED` - 连接确认
- `MESSAGE` - 消息传递
- `RECEIPT` - 收据确认
- `ERROR` - 错误响应

### 版本差异

#### STOMP 1.0
- 基础协议功能
- 简单的头部处理
- 可选的订阅ID

#### STOMP 1.1
- 协议版本协商
- 心跳机制
- NACK命令
- 虚拟主机支持
- 头部转义

#### STOMP 1.2
- 简化的消息确认
- 改进的头部处理
- 连接保持机制

## 配置选项

### 服务器配置

```go
type StompServer struct {
    upgrader    websocket.Upgrader  // WebSocket升级器配置
    // ... 其他字段
}

// 自定义WebSocket升级器
server := NewStompServer()
server.upgrader.CheckOrigin = func(r *http.Request) bool {
    // 自定义跨域检查逻辑
    return true
}
```

## 与GoFrame集成

### 数据库集成

```go
stompServer.SetMessageHandler("/topic/database", func(conn *stomp.Connection, frame *stomp.Frame) error {
    // 使用GoFrame ORM保存消息
    _, err := g.DB().Insert("messages", g.Map{
        "destination": "/topic/database",
        "content":     string(frame.Body),
        "created_at":  time.Now(),
    })
    return err
})
```

### 缓存集成

```go
stompServer.SetMessageHandler("/topic/cache", func(conn *stomp.Connection, frame *stomp.Frame) error {
    // 使用GoFrame缓存
    cacheKey := fmt.Sprintf("stomp:message:%d", time.Now().UnixNano())
    return g.Redis().Set(context.Background(), cacheKey, string(frame.Body), time.Hour)
})
```

### 中间件集成

```go
s := g.Server()

// 添加CORS中间件
s.Use(func(r *ghttp.Request) {
    r.Response.CORSDefault()
    r.Middleware.Next()
})

// 添加日志中间件
s.Use(ghttp.MiddlewareHandlerResponse)
```

## 错误处理

### 服务器端错误

```go
// 发送错误给客户端
conn.SendError("Invalid destination", "Destination '/invalid' not found")

// 记录错误日志
g.Log().Error(context.Background(), "STOMP error:", err)
```

## 性能优化

### 连接池

```go
// 使用连接池管理客户端连接
type ClientPool struct {
    clients chan *stomp.Client
    config  *stomp.ClientConfig
}

func (p *ClientPool) Get() *stomp.Client {
    select {
    case client := <-p.clients:
        return client
    default:
        client := stomp.NewClient(p.config)
        client.Connect(p.config)
        return client
    }
}
```

### 消息缓冲

```go
// 增加消息通道缓冲区大小
client := stomp.NewClient(config)
// 内部已设置合理的缓冲区大小
```

## 安全考虑

### 认证

```go
// 在消息处理器中验证用户权限
stompServer.SetMessageHandler("/secure/topic", func(conn *stomp.Connection, frame *stomp.Frame) error {
    // 验证用户是否有权限访问此主题
    if !hasPermission(conn.sessionID, "/secure/topic") {
        return fmt.Errorf("access denied")
    }
    // 处理消息
    return nil
})
```

### 跨域设置

```go
server := stomp.NewStompServer()
server.upgrader.CheckOrigin = func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    // 检查允许的源
    return isAllowedOrigin(origin)
}
```

## 参考资料

- [STOMP 1.0 规范](https://stomp.github.io/stomp-specification-1.0.html)
- [STOMP 1.1 规范](https://stomp.github.io/stomp-specification-1.1.html)
- [STOMP 1.2 规范](https://stomp.github.io/stomp-specification-1.2.html)
- [Gorilla WebSocket](https://github.com/gorilla/websocket)