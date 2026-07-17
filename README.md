# nscan 正式部署包

本目录包含 nscan 前端、后端、扫描器源码及全部 Docker 部署文件。
不需要依赖项目根目录的其他开发资料。

## 部署

```bash
cd nscan_production
cp deploy/.env.production.example deploy/.env.production
```

填写域名后执行：

```bash
./deploy/install.sh
```

Docker Nginx 配置文件：

```text
deploy/nginx.conf
```

## 管理

查看状态：

```bash
./deploy/healthcheck.sh
```

查看日志：

```bash
docker compose --env-file deploy/.env.production \
  -f deploy/docker-compose.prod.yaml logs -f
```

停止服务：

```bash
docker compose --env-file deploy/.env.production \
  -f deploy/docker-compose.prod.yaml down
```

启动服务：

```bash
./deploy/install.sh
```

备份数据：

```bash
./deploy/backup.sh
```

升级版本：

```bash
./deploy/upgrade.sh 1.0.1
```

回滚版本：

```bash
./deploy/rollback.sh 1.0.0
```
