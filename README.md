# STOMP WebSocket Server for GoFrame

这是一个基于WebSocket的STOMP（Simple Text Oriented Messaging Protocol）服务器实现，专为GoFrame框架设计，兼容STOMP 1.0、1.1和1.2版本规范。

## 特性

- ✅ **多版本支持**: 兼容STOMP 1.0、1.1、1.2规范
- ✅ **WebSocket集成**: 基于gorilla/websocket实现
- ✅ **GoFrame集成**: 与GoFrame框架无缝集成
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

### 2. 使用STOMP客户端

```go
package main

import (
    "time"
    "your-project/utility/stomp"
)

func main() {
    // 创建客户端配置
    config := &stomp.ClientConfig{
        URL:            "ws://localhost:8080/stomp",
        Login:          "guest",
        Passcode:       "guest",
        Version:        "1.2",
        ConnectTimeout: 30 * time.Second,
    }
    
    // 创建并连接客户端
    client := stomp.NewClient(config)
    err := client.Connect(config)
    if err != nil {
        panic(err)
    }
    defer client.Disconnect()
    
    // 订阅主题
    messageChan, err := client.Subscribe("/topic/chat", "auto")
    if err != nil {
        panic(err)
    }
    
    // 监听消息
    go func() {
        for message := range messageChan {
            fmt.Printf("Received: %s\n", string(message.Body))
        }
    }()
    
    // 发送消息
    err = client.Send("/topic/chat", []byte("Hello, STOMP!"), nil)
    if err != nil {
        panic(err)
    }
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

#### Connection

```go
// 发送帧到客户端
func (c *Connection) SendFrame(frame *Frame) error

// 发送错误帧
func (c *Connection) SendError(message, description string)
```

### 客户端

#### Client

```go
// 创建新客户端
func NewClient(config *ClientConfig) *Client

// 连接到服务器
func (c *Client) Connect(config *ClientConfig) error

// 断开连接
func (c *Client) Disconnect() error

// 发送消息
func (c *Client) Send(destination string, body []byte, headers map[string]string) error

// 订阅主题
func (c *Client) Subscribe(destination, ack string) (<-chan *Frame, error)

// 取消订阅
func (c *Client) Unsubscribe(destination string) error

// 确认消息
func (c *Client) Ack(messageID string) error

// 拒绝消息 (STOMP 1.1+)
func (c *Client) Nack(messageID string) error
```

### Frame

```go
// 创建新帧
func NewFrame(command string) *Frame

// 解析帧
func ParseFrame(data []byte) (*Frame, error)

// 序列化帧
func (f *Frame) Marshal() []byte

// 设置头部
func (f *Frame) SetHeader(key, value string)

// 获取头部
func (f *Frame) GetHeader(key string) (string, bool)

// 设置消息体
func (f *Frame) SetBody(body []byte)

// 验证帧
func (f *Frame) Validate() error
```

## STOMP协议支持

### 支持的命令

#### 客户端命令
- `CONNECT` / `STOMP` - 建立连接
- `SEND` - 发送消息
- `SUBSCRIBE` - 订阅目标
- `UNSUBSCRIBE` - 取消订阅
- `ACK` - 确认消息
- `NACK` - 拒绝消息 (1.1+)
- `BEGIN` - 开始事务
- `COMMIT` - 提交事务
- `ABORT` - 中止事务
- `DISCONNECT` - 断开连接

#### 服务器命令
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

### 客户端配置

```go
type ClientConfig struct {
    URL             string        // WebSocket URL
    Login           string        // 登录用户名
    Passcode        string        // 登录密码
    Version         string        // 协议版本
    Heartbeat       string        // 心跳配置
    ConnectTimeout  time.Duration // 连接超时
    MessageTimeout  time.Duration // 消息超时
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

### 客户端错误

```go
// 处理连接错误
if err := client.Connect(config); err != nil {
    log.Printf("Connection failed: %v", err)
}

// 处理发送错误
if err := client.Send(destination, message, nil); err != nil {
    log.Printf("Send failed: %v", err)
}
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

## 测试

### 单元测试示例

```go
func TestStompServer(t *testing.T) {
    server := stomp.NewStompServer()
    
    // 测试消息处理器
    server.SetMessageHandler("/test", func(conn *stomp.Connection, frame *stomp.Frame) error {
        return nil
    })
    
    // 测试发送消息
    err := server.SendToDestination("/test", []byte("test"), nil)
    assert.NoError(t, err)
}
```

## 故障排除

### 常见问题

1. **连接失败**
   - 检查WebSocket URL是否正确
   - 确认服务器是否运行
   - 检查防火墙设置

2. **消息丢失**
   - 检查订阅是否成功
   - 确认消息处理器是否正确设置
   - 检查网络连接稳定性

3. **性能问题**
   - 增加消息缓冲区大小
   - 使用连接池
   - 优化消息处理逻辑

### 调试

```go
// 启用详细日志
g.Log().SetLevel(glog.LEVEL_ALL)

// 监控连接状态
fmt.Printf("Connected: %v\n", client.IsConnected())
fmt.Printf("Version: %s\n", client.GetVersion())
fmt.Printf("Session: %s\n", client.GetSessionID())
```

## 许可证

本项目采用MIT许可证。详见LICENSE文件。

## 贡献

欢迎提交Issue和Pull Request来改进这个项目。

## 参考资料

- [STOMP 1.0 规范](https://stomp.github.io/stomp-specification-1.0.html)
- [STOMP 1.1 规范](https://stomp.github.io/stomp-specification-1.1.html)
- [STOMP 1.2 规范](https://stomp.github.io/stomp-specification-1.2.html)
- [GoFrame 文档](https://goframe.org/)
- [Gorilla WebSocket](https://github.com/gorilla/websocket)