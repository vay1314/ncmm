# 🐳 Docker 部署指南

本项目已提供完整的 Docker 镜像构建配置和 Docker Compose 快捷托管方案，且支持**完全开箱即用**。运行时所需的 `config.yaml` 默认配置文件会在容器首次启动时自动在宿主机挂载目录下生成。

---

## 1. 使用 Docker Compose 部署 (推荐)

在项目根目录下，直接配置 `docker-compose.yml` 如下：

```yaml
services:
  ncmm:
    build: .
    image: ghcr.io/3899/ncmm:latest
    container_name: ncmm
    # 启用定时任务时，可将重启策略设为 always 或 unless-stopped 保证后台常驻
    restart: "no"
    network_mode: host
    volumes:
      - ./data:/data
    environment:
      - TZ=Asia/Shanghai
      # CookieCloud 配置（按实际情况修改）
      - COOKIECLOUD_SERVER=http://127.0.0.1:8088
      - COOKIECLOUD_UUID=your-uuid
      - COOKIECLOUD_PASSWORD=your-password

      # 方式一：使用环境变量配置定时任务规则（支持多重配置，支持的命令详见文档 docs\cli.md）
      - CRON_1=30 8 * * * task
      # - CRON_2=30 10 * * * sign
      # - CRON_3=0 14 * * * musician
      # - CRON_4=0 22 * * * playids --ids 12345

    # 方式二：使用 command 传参配置定时任务规则（推荐，支持一行写一条命令，直接加 # 注释）
    # command:
    #   - cron
    #   - |
    #     30 8 * * * task
    #     30 10 * * * sign
    #     0 14 * * * musician
    #     0 22 * * * playids --ids 123456
```

### 后台定时托管运行：
1. 启用并配置定时规则（推荐使用上方 `command` 的方式二，或者 `CRON_X` 环境变量的方式一）。
2. 将 `restart` 策略修改为 `always`。
3. 运行以下命令拉起容器：
   ```bash
   docker compose up -d
   ```
   首次启动后，宿主机 `./data` 目录下会自动释放 `config.yaml`。任何登录回写的 Cookie、日志和 Badger 数据库缓存也会自动保存在 `./data` 目录中。

### 临时单次运行特定任务：
不启用任何 `CRON` 变量或 `command`，直接通过命令行临时拉起特定子任务：
```bash
docker compose run --rm ncmm task                      # 批量运行所有配置任务
docker compose run --rm ncmm sign                      # 仅运行日常一键签到
docker compose run --rm ncmm playids --ids 3366663042  # 仅播放指定歌曲
docker compose run --rm ncmm musician                  # 仅运行音乐人任务
docker compose run --rm ncmm --help                    # 打印帮助手册
```

---

## 2. 使用 Docker 命令行部署 (`docker run`)

你也可以使用 `docker run` 命令行单次拉起或以环境变量配置定时托管运行：

### 单次测试运行：
```bash
docker run --rm -v ./data:/data ghcr.io/3899/ncmm:latest task
```

### 后台常驻定时托管（每天 8:30 签到，14:00 做音乐人任务）：
```bash
docker run -d --name ncmm \
  --restart always \
  --network host \
  -v ./data:/data \
  -e TZ=Asia/Shanghai \
  -e "CRON_1=30 8 * * * sign" \
  -e "CRON_2=0 14 * * * musician" \
  ncmm:latest
```
