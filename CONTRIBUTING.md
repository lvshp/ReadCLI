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
git clone https://github.com/lvshp/ReadCLI.git
cd ReadCLI
go test ./...
go build -o readcli ./cmd
./readcli -n 8 /path/to/book.epub
```

## 发布说明

### 开发测试版

推送到 `dev` 后，Release 工作流先测试，再从该次推送的提交构建 macOS arm64/amd64、Linux amd64 和 Windows amd64 安装包，自动发布为 **Pre-release**，不占用稳定版的 **Latest** 标志。

- 版本号形如 `v0.4.1-dev.42.1`，末两段是 Actions 运行编号和准备步骤的重试次数；重新运行全部任务生成新版本，只重试失败任务则继续原次发布。
- 下一正式版本号维护在 `.github/prerelease-version`，开发版说明维护在 `.github/release-notes/dev.md`。每次开发版包含该提交的完整说明，并附上提交和构建记录。
- 也可在 Actions → Release → Run workflow 选择 `dev` 手动构建；选择其他分支会失败，避免误发布。
- 测试版从 Releases 手动下载。应用内更新继续使用 GitHub 的稳定版接口，不分发预发布包。

首次启用时，先将 `.github/` 中的发布配置与脚本同步到默认分支 `main`，再推送 `dev`，确保两边的工作流文件相同。GitHub 的 Releases API 对目标提交相对默认分支修改工作流的情况要求额外的 Workflows 写权限，普通 `GITHUB_TOKEN` 不具备该权限；手动运行入口也需要工作流已存在于默认分支。之后修改工作流时也要同步默认分支。参见 [GitHub Releases API](https://docs.github.com/en/rest/releases/releases) 和 [手动运行工作流](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow)。

### 正式版

先合并到 `main`，再推送 `vMAJOR.MINOR.PATCH` 格式的正式 tag。工作流会验证提交已包含在 `main`，测试通过后发布多平台附件及 SHA-256 校验文件。

正式版说明优先使用 annotated tag 的注释；没有注释时使用 `CHANGELOG.md` 对应版本段落。缺少说明会终止发布。带 `-dev`、`-beta` 等后缀的 tag 不触发正式发布。
