# 贡献指南

欢迎为这个维护版提交 Issue 和 Pull Request。

## 适合贡献的方向

- EPUB 兼容性改进
- 更多电子书格式支持
- 终端界面与伪装风格优化
- 性能与排版优化
- Windows 支持
- 测试覆盖完善

## 提交 Issue 前

建议尽量提供这些信息：

- 使用的平台与架构
- 使用的命令
- 输入文件格式，例如 `txt` 或 `epub`
- 实际现象
- 预期行为
- 如果是界面问题，最好附截图

## 提交 Pull Request 前

请尽量遵循下面的简单约定：

- 保持改动聚焦，不把无关重构混进同一个 PR
- 如果改了行为，尽量补测试
- 如果改了使用方式，记得同步更新 README
- 保留原项目协议与致谢信息

## 本地开发

```bash
git clone https://github.com/lvshp/glance.git
cd glance
go test ./...
go build -o glance ./cmd
./glance -n 8 /path/to/book.epub
```

## 发布说明

当前仓库已经配置了自动 Release 工作流。

推送形如 `v0.1.1` 的 tag 后，GitHub Actions 会自动构建并上传多平台附件。
