# ReadCLI

[中文说明](./README.md) | [English](./README_EN.md)

[![Release](https://img.shields.io/github/v/release/lvshp/ReadCLI?label=%E6%9C%80%E6%96%B0%E7%89%88%E6%9C%AC)](https://github.com/lvshp/ReadCLI/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/lvshp/ReadCLI/go.yml?branch=main&label=CI)](https://github.com/lvshp/ReadCLI/actions/workflows/go.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)

ReadCLI 是一个终端阅读器，支持 `TXT` 和 `EPUB`，带本地书架、进度保存、书签、搜索，以及几套偏 IDE 风格的界面。基于 [tcell](https://github.com/gdamore/tcell) 和 [tview](https://github.com/rivo/tview) 构建，支持 macOS、Linux 和 Windows。

## 页面展示

### 书架首页

![书架首页](./demo/bookshelf-home.png)

### 阅读界面

![阅读界面](./demo/reading-view.png)

### 目录导航

![目录导航](./demo/toc-view.png)

### 书签列表

![书签列表](./demo/bookmarks-view.png)

### 搜索输入

![搜索输入](./demo/search-input.png)

### 导入界面

![导入界面](./demo/import-input.png)

## 功能

### 阅读

* 支持 `TXT`
* 支持 `EPUB`
* 自动恢复上次阅读位置
* 支持章节目录、上一章 / 下一章跳转
* 支持全文搜索，`n / N` 跳转结果
* 支持当前页搜索结果高亮
* 支持书签保存、查看、删除、跳转
* 支持按页滚动和自定义每页显示行数
* 支持全信息阅读界面和精简阅读界面切换，精简模式只保留正文、章节和进度信息
* 支持自定义正文宽度、上下左右边距、顶部留白、行间距、字体颜色、高对比、基础色模式和自动翻页间隔
* 支持按终端宽度和中文字符显示宽度自适应折行
* 支持阅读器缓存，并会根据文件大小和修改时间自动失效，避免文件更新后读到旧内容
* 跨平台支持：macOS、Linux、Windows Terminal

### 书架

* 无参数启动进入书架首页
* 支持单本导入和目录批量导入
* 目录导入默认只扫描当前层，可切换为递归导入
* 记录每本书的格式、章节、进度和最近阅读时间
* 支持按最近阅读、导入时间、书名排序
* 支持按格式和阅读状态过滤
* 支持仅移出书架，或连本地文件一起删除
* 书架主列表聚焦书名，右侧详情块展示格式、状态、进度、章节和最近阅读时间

### 界面和交互

* 支持 `vscode`、`jetbrains`、`ops-console` 三套主题
* 书架、阅读、目录、书签都有独立状态栏和操作提示
* 阅读界面支持 `z` 在全信息 / 精简模式之间切换，并会保存到配置
* 导入路径支持光标移动、`Tab` 补全、候选选择和分页显示
* 支持把文件或目录直接拖到导入输入框里自动识别路径
* 同时兼容 Vim 键位和普通方向键
* 启动时会自动检查 GitHub Releases 更新，并显示当前版本号
* 支持手动检查更新、查看更新说明并热更新，下载时显示进度、大小和安装阶段，更新后退出重启生效
* 支持 Boss Key、自定义老板键命令和自动翻页
* 退出时会统一持久化书架位置、阅读样式和最近阅读状态

## 终端兼容

ReadCLI 基于 tcell v2 构建，原生支持 Unicode 边框和 CJK 字符宽度，跨平台兼容性良好。

### macOS

* `iTerm2`
* `Terminal.app`
* `WezTerm`
* `Alacritty`
* `Kitty`

### Linux

* `gnome-terminal`
* `kitty`
* `wezterm`
* `alacritty`
* `xterm`
* `tmux` / `screen`

### Windows

* `Windows Terminal`
* `PowerShell` 7+（终端宿主）

需要注意的地方主要是两类：

* 颜色显示能力取决于终端是否支持 `256 色` 或更高色彩模式
* 拖入文件或目录到输入框，取决于终端模拟器本身是否支持拖拽路径

如果某些 Linux 终端里自定义颜色发灰、偏色或不够清楚，可以在 `config.json` 里打开基础色模式：

```json
{
  "force_basic_color": true
}
```

开启后会强制退回 ANSI 基础色，牺牲一点颜色精度，换取更稳的跨终端显示效果。

## Fork 说明

这个仓库基于原项目 [TimothyYe/glance](https://github.com/TimothyYe/glance) 修改而来。

这个分支保留了原项目的许可证和基础思路，并继续维护到现在这版功能。

如果你想使用原项目，可以前往：
[https://github.com/TimothyYe/glance](https://github.com/TimothyYe/glance)

## 下载与安装

预编译包：

* [Releases 页面](https://github.com/lvshp/ReadCLI/releases)

当前已提供：

* macOS arm64
* macOS amd64
* Linux amd64
* Windows amd64

从源码构建：

```bash
git clone https://github.com/lvshp/ReadCLI.git
cd ReadCLI
go build -o readcli ./cmd
```

> 需要 Go 1.24+

交叉编译 Windows 版本：

```bash
GOOS=windows GOARCH=amd64 go build -o readcli.exe ./cmd
```

如果二进制已经在 `PATH` 里，直接运行：

```bash
readcli
```

如果想全局使用 `readcli`，可以把二进制放到 `~/.local/bin`，并确保它在 `PATH` 里：

```bash
mkdir -p ~/.local/bin
cp ./readcli ~/.local/bin/readcli
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

然后验证：

```bash
command -v readcli
```

正常情况下会输出类似：

```bash
/Users/your-name/.local/bin/readcli
```

或者直接打开一本书：

```bash
readcli /path/to/book.epub
readcli /path/to/book.txt
```

## 快速开始

### 1. 打开书架

```bash
readcli
```

进入书架后可以用 `i` 导入本地书籍，用 `↑/↓` 或 `j/k` 选择，用 `→` 或 `Enter` 打开。

### 2. 直接打开一本书

```bash
readcli -n 8 /path/to/book.epub
```

会自动把书加入书架，按上次位置恢复阅读，并用每页 `8` 行显示正文。

### 3. 导入文件

导入界面支持路径补全、候选选择、拖入文件或目录，以及 `Ctrl-r` 切换当前层导入 / 递归导入。

### 4. 调整阅读样式

阅读时按 `,` 可以打开阅读设置，直接修改：

* 正文宽度
* 上下左右边距
* 行间距
* 字体颜色
* 高对比模式
* 基础色模式
* 自动翻页间隔

按 `c` 可以快速切换一组亮色预设。
按 `t` 可以开关自动翻页。
按 `z` 可以在全信息阅读界面和精简阅读界面之间切换；精简模式会隐藏头部、侧栏和大部分提示，只保留小说正文、章节名称和阅读进度。

### 5. 检查更新

启动后会自动检查新版本。检测到更新时，会弹出更新说明和确认界面。

如果自动弹窗里选择这次不更新，这个版本以后启动时不会再自动提醒；如果之后想手动更新，可以随时按 `u` 重新检查。

确认更新后，程序会下载对应平台的发布包并替换当前二进制。更新过程中会显示下载百分比、进度条、已下载大小和安装阶段。更新完成后退出，再重新启动即可生效。

Windows 会先将完整的新程序暂存到安装目录，显示“更新文件已准备”后，按 Enter 退出，由更新助手完成替换，再重新启动。安装在 `Program Files` 等受保护目录时会请求管理员授权；取消授权则保留旧程序。更新失败时可滚动查看完整原因和保留文件的位置。0.3.5/0.3.6 的旧更新器如遇目录权限或跨盘问题，需要先退出程序，手动覆盖一次带修复的版本。

### 6. 配置 Boss Key

默认情况下，按 `b` 会切到内置的伪装页面。

如果你想按老板键时直接运行一条外部命令，在 `config.json` 里设置 `boss_key_command`：

```json
{
  "boss_key_command": "genact"
}
```

也可以写成带参数的完整命令。

macOS / Linux 示例：

```json
{
  "boss_key_command": "/usr/local/bin/genact -m terraform"
}
```

Windows 示例：

```json
{
  "boss_key_command": "genact.exe"
}
```

配置后，按 `b` 会临时退出 ReadCLI 的界面，在当前终端里执行这条命令；退出该命令后，会自动回到 ReadCLI。

注意：执行命令的前提是你本地已经安装并可运行此命令。

推荐安装：[svenstaro/genact](https://github.com/svenstaro/genact)

## 键位说明

按 `?` 可以打开内置帮助页。支持 Vim 风格键位和方向键。

### 书架首页

* Vim 风格：`j/k` 移动，`Enter` 打开，`i` 导入，`o/r` 排序过滤，`x` 移除，`u` 检查更新
* 普通键位：`↑/↓` 移动，`→` 或 `Enter` 打开，`u` 检查更新

### 阅读界面

* Vim 风格：`j/k` 翻页，`[` / `]` 切章，`/` 搜索，`n/N` 跳转结果，`s/B` 书签，`m` 目录，`c` 切换颜色，`z` 精简/全信息，`u` 检查更新
* 普通键位：`↑/↓` 翻页，`←/→` 切章，`z` 精简/全信息，`u` 检查更新
* 阅读设置：按 `,` 打开设置面板，可调整正文对齐、正文宽度、边距、行间距、字体颜色、高对比、基础色模式和自动翻页间隔

### 目录 / 书签

* 目录支持数字跳章，目录标题已改成中文显示，不再显示左侧序号
* Vim 风格：`j/k` 移动，`Enter` 打开
* 普通键位：`↑/↓` 移动，`→` 或 `Enter` 打开，`←` 返回

### 通用操作

* `+ / -` 调整每页显示行数
* `a` 在阅读界面循环切换正文区域：居中 → 居左 → 居右，立即生效并保存
* `c` 快速切换正文颜色
* `T` 切换主题
* `z` 切换精简 / 全信息阅读界面
* `u` 手动检查更新
* `p` 查看阅读进度
* `b` 触发 Boss Key
* `f` 显示或隐藏边框
* `q` 返回书架或退出程序

## 数据保存位置

本地数据默认保存在：

* macOS / Linux：`~/.readcli/`
* Windows：`%USERPROFILE%\.readcli\`（通常为 `C:\Users\<用户名>\.readcli\`）

也可以通过环境变量 `READCLI_DATA_DIR` 指定自定义路径。

目录里会有：

* `config.json`
* `bookshelf.json`
* `bookmarks.json`
* `progress.json`

其中 `config.json` 里会保存阅读样式相关设置，例如：

* 正文宽度比例
* 上下左右边距
* 老板键自定义命令
* 行间距
* 自动翻页间隔（毫秒）
* 字体颜色（支持 `#RRGGBB`、`#RGB`、`R,G,B`）
* `reading_alignment`：正文区域对齐，可设为 `"center"`（居中）、`"left"`（居左）、`"right"`（居右）
* `compact_mode`：是否使用精简阅读界面
* `reading_high_contrast`：控制正文和侧栏的高对比显示
* `force_basic_color`：强制使用基础 ANSI 颜色，适合颜色支持较弱的终端

例如，在 `config.json` 中加入 `"reading_alignment": "right"`，下次启动后正文区域将居右。也可以按 `,` 打开阅读设置，选择“正文对齐”，用 `←/→` 或 `Enter` 切换。精简和全信息模式共用此设置，切换不会改变正文换行或阅读进度。对齐移动的是整块正文区域，段落缩进保持不变；左右边距作为对齐时的留白。若要贴最左侧显示，可设置 `"reading_alignment": "left"` 和 `"reading_margin_left": 0`。

未配置该字段、留空或填写无效值时，沿用原布局：精简模式居中，全信息模式居左。

## 开发和贡献

欢迎提 issue 和 PR。

相关文档：

* [CHANGELOG](./CHANGELOG.md)
* [CONTRIBUTING](./CONTRIBUTING.md)
* [DEVELOPMENT_PLAN](./DEVELOPMENT_PLAN.md)

## 友链

* [linux.do](https://linux.do/)

## 协议

项目继续遵循 [Apache License 2.0](./LICENSE)。
