# 作业 6：后端（api）是否暴露给用户？浏览器如何访问后端接口

> 分析对象：本人学期仓 `docker-compose.yml`、`deploy/nginx.conf`、`backend/Dockerfile`、`.env.example`。

## 一、结论先行

**后端 api 没有直接暴露给用户。** 三个容器中只有 web（Nginx）把端口映射到宿主机（`0.0.0.0:8080->80`）；api 的 8081 与 db 的 3306 都只在 Compose 内部网络可达，没有 `ports:` 映射。用户在浏览器里**无法**通过 `localhost:8081` 直连后端；访问后端接口的唯一方式是**同源反向代理**——浏览器请求 `http://localhost:8080/api/...`，由 Nginx 转发给内部网络的 api 容器。

## 二、配置层面的三条证据（对应作业要求的分析路径）

### 证据 1：docker-compose.yml —— api 服务没有 `ports:` 映射

```yaml
api:
  build: ./backend
  environment: ...
  volumes:
    - uploads:/app/uploads
  depends_on:
    - db
  # 注意：整个 api 服务没有 ports: 字段 —— 不映射任何宿主端口
```

对照 web 服务：`ports: ["${WEB_PORT:-8080}:80"]`，是三容器中**唯一**的宿主端口映射。db 同样无 `ports:`（MySQL 3306 不出内部网络）。

### 证据 2：deploy/nginx.conf —— 后端接口经同源反代到达

```nginx
# 前端 SPA：页面路由回落 index.html
location / {
    try_files $uri $uri/ /index.html;
}
# API 反代：/api 开头的请求转发到内部网络的 api 容器
location /api/ {
    proxy_pass http://api:8081;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    client_max_body_size 2m;
}
location = /health {
    proxy_pass http://api:8081/health;
}
```

`proxy_pass http://api:8081` 中的 `api` 是 Compose 内部网络的服务名，只在容器间可解析——浏览器永远接触不到这个地址。

### 证据 3：.env.example —— 端口的分工

```
WEB_PORT=8080      # 唯一映射到宿主机的端口（用户入口）
# api 的 API_PORT=8081 只在容器内部监听，不配映射
```

## 三、运行层面的实测验证

`docker compose ps` 的 PORTS 列直观呈现暴露面：

```
NAME                 PORTS
internetdemo-web-1   0.0.0.0:8080->80/tcp      ← 唯一映射到宿主机
internetdemo-api-1   8081/tcp                   ← 仅容器端口，无 "->" 映射
internetdemo-db-1    3306/tcp, 33060/tcp        ← 仅容器端口，无 "->" 映射
```

直连与反代对照（宿主机执行）：

| 操作 | 结果 | 说明 |
| --- | --- | --- |
| `curl http://localhost:8081/health` | 连接失败（Connection refused） | api 端口未映射，浏览器/终端都够不到 |
| `curl http://localhost:8080/health` | `{"status":"ok"}` | 经 Nginx 反代到达 api，无需登录 |
| `curl http://localhost:8080/api/materials` | `401 {"error":"未登录"}` | 反代到达 api，但鉴权在中间件拦截 |

## 四、这样设计的目的（攻击面最小化）

1. **信任边界唯一**：浏览器只能摸到 Nginx 一个入口，认证、授权、班级隔离、上传校验全部发生在反代之后的 Go 进程里；前端与静态页一律不可信。
2. **减少探测入口**：能扫到的端口越少，越权探测的入口越少——db 不映射 3306，连数据库排障都要 `docker compose exec` 进容器做。
3. **同源免跨域**：前端以同源 `/api` 调用后端，规避了跨域请求的 Cookie 作用域与 SameSite 问题（开发环境 Vite 代理路径与生产 Nginx 保持一致）。
