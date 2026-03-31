# ReadCLI

[![Release](https://img.shields.io/github/v/release/lvshp/ReadCLI?label=%E6%9C%80%E6%96%B0%E7%89%88%E6%9C%AC)](https://github.com/lvshp/ReadCLI/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/lvshp/ReadCLI/go.yml?branch=main&label=CI)](https://github.com/lvshp/ReadCLI/actions/workflows/go.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)

ReadCLI 是一个终端小说阅读器和本地书架工具，主打三件事：

* 在终端里舒服地阅读 `TXT` / `EPUB`
* 自动管理书架、阅读进度、书签和搜索
* 用更像 IDE / 工作台的界面低调阅读

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

## 这是什么

和原始版本相比，这个项目已经不只是“打开一本 txt 往下翻”的小工具，而是更接近一个完整的终端阅读工作台：

* 无参数启动直接进入书架首页
* 支持导入本地 `txt` / `epub`
* 每本书单独保存阅读进度
* 支持章节目录、章节切换、正文搜索和书签
* 支持多主题伪装界面
* 支持 Vim 风格键位和普通方向键两套操作方式

适合这些场景：

* 想在终端里读网文、小说、技术文档
* 想保留键盘流和轻量体验
* 想在办公环境里把界面伪装得更像开发工具
* 想要一个还在持续维护的 `glance` 分支

## 当前特性

### 阅读能力

* 支持 `TXT`
* 支持 `EPUB`
* 自动恢复上次阅读位置
* 支持章节目录、上一章 / 下一章跳转
* 支持全文搜索，`n / N` 跳转结果
* 支持当前页搜索结果高亮
* 支持书签保存、查看、删除、跳转
* 支持按页滚动和自定义每页显示行数
* 支持按终端宽度和中文字符显示宽度自适应折行

### 书架能力

* 无参数启动进入书架首页
* 支持单本导入和目录批量导入
* 目录导入默认只扫描当前层，也可以切换为递归导入
* 记录每本书的格式、章节、进度和最近阅读时间
* 支持按最近阅读、导入时间、书名排序
* 支持按格式和阅读状态过滤
* 支持仅移出书架，或连本地文件一起删除
* 书架主列表聚焦书名，右侧详情块展示格式、状态、进度、章节和最近阅读时间
* 优先从正文或 EPUB 元数据提取书名，提取不到再回退到文件名
* 自动清理常见来源尾巴，例如 `Z-Library`

### 交互与界面

* 支持 `vscode`、`jetbrains`、`ops-console` 三套主题
* 首页、阅读页、目录、书签页都有独立状态栏和操作提示
* 导入路径支持光标移动、`Tab` 补全、候选选择和分页显示
* 支持把文件或目录直接拖到导入输入框里自动识别路径
* 同时兼容 Vim 键位和普通方向键
* 支持 Boss Key 和自动翻页

## Fork 说明

这个仓库基于原项目 [TimothyYe/glance](https://github.com/TimothyYe/glance) 修改而来。

这个维护分支保留了原项目的许可证、致谢和基础理念，并在此基础上继续演进。当前新增和增强的方向主要包括：

* EPUB 阅读与章节导航
* 本地书架和进度持久化
* 搜索、书签、多行阅读和按页滚动
* 中文排版与终端宽度自适应
* 更完整的 IDE / 工作台伪装 UI

如果你想使用原项目，可以前往：
[https://github.com/TimothyYe/glance](https://github.com/TimothyYe/glance)

## 下载与安装

发布版本与预编译包：

* [Releases 页面](https://github.com/lvshp/ReadCLI/releases)

当前已提供：

* macOS arm64
* macOS amd64
* Linux amd64

从源码构建：

```bash
git clone https://github.com/lvshp/ReadCLI.git
cd ReadCLI
go build -o readcli ./cmd
```

如果你已经把可执行文件放到 `PATH` 中，可以直接使用：

```bash
readcli
```

如果你想全局直接使用 `readcli`，可以把二进制放到 `~/.local/bin`，并确保它在 `PATH` 里：

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

进入书架首页后：

* `i` 导入本地书籍
* `↑/↓` 或 `j/k` 选择书籍
* `→` 或 `Enter` 打开

### 2. 直接打开一本书

```bash
readcli -n 8 /path/to/book.epub
```

这会：

* 自动把书加入书架
* 按上次位置恢复阅读
* 以每页 `8` 行的方式显示正文

### 3. 导入本地文件

导入界面支持：

* `←/→` 移动光标
* `Backspace/Delete` 删除字符
* `Tab` 路径补全
* `↑/↓` 或继续按 `Tab` 选择候选
* 支持把文件或目录直接拖进输入框
* `Ctrl-r` 切换当前层导入 / 递归导入
* `Enter` 先填入候选，再执行导入

## 键位说明

按 `?` 可以打开内置帮助页。当前支持两套操作方式。

### 书架首页

* Vim 风格：`j/k` 移动，`Enter` 打开，`i` 导入，`o/r` 排序过滤，`x` 移除
* 普通键位：`↑/↓` 移动，`→` 或 `Enter` 打开

### 阅读界面

* Vim 风格：`j/k` 翻页，`[` / `]` 切章，`/` 搜索，`n/N` 跳转结果，`s/B` 书签，`m` 目录
* 普通键位：`↑/↓` 翻页，`←/→` 切章

### 目录 / 书签

* Vim 风格：`j/k` 移动，`Enter` 打开
* 普通键位：`↑/↓` 移动，`→` 或 `Enter` 打开，`←` 返回

### 通用操作

* `+ / -` 调整每页显示行数
* `T` 切换主题
* `p` 查看阅读进度
* `b` 触发 Boss Key
* `f` 显示或隐藏边框
* `q` 返回书架或退出程序

## 数据保存位置

默认会把本地数据保存在用户根目录下的隐藏目录 `.readcli` 中，包括：

* `config.json`
* `bookshelf.json`
* `bookmarks.json`
* `progress.json`

默认位置在用户根目录下：

```bash
~/.readcli/
```

## 项目定位

ReadCLI 现在的重点不是做一个“重型电子书管理器”，而是做一个：

* 启动快
* 终端原生
* 键盘友好
* 足够好看
* 足够适合日常摸鱼阅读

后续更适合继续增强的方向包括：

* 更丰富的主题和伪装模式

## 开发与贡献

欢迎提交 issue 和 PR。

相关文档：

* [CHANGELOG](./CHANGELOG.md)
* [CONTRIBUTING](./CONTRIBUTING.md)

## 协议

本项目继续遵循 [Apache License 2.0](./LICENSE)。
