# internal/

每个 service 独立一个子目录，分层：

```
internal/<service>/
├── config/   # Viper + struct
├── server/   # HTTP (Gin) / gRPC / WS 入口与路由
├── service/  # 业务用例（use-case）
├── repo/     # 数据访问（MySQL/Redis/Kafka）
└── model/    # 领域模型 + DTO
```

下一轮会先铺 `user-service`（登录 + JWT + refresh 轮换），
然后 `room-service`（房间列表 + 关注 + 点赞），最后对齐前端 MSW 行为自查。
