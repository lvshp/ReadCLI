# ReadCLI

[中文说明](./README.md) | [English](./README_EN.md)

[![Release](https://img.shields.io/github/v/release/lvshp/ReadCLI?label=Latest%20Release)](https://github.com/lvshp/ReadCLI/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/lvshp/ReadCLI/go.yml?branch=main&label=CI)](https://github.com/lvshp/ReadCLI/actions/workflows/go.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)

ReadCLI is a terminal ebook reader with support for `TXT` and `EPUB`, plus a local bookshelf, saved progress, bookmarks, search, and several IDE-style themes.

## Screenshots

### Bookshelf

![Bookshelf](./demo/bookshelf-home.png)

### Reading View

![Reading View](./demo/reading-view.png)

### Table of Contents

![TOC](./demo/toc-view.png)

### Bookmarks

![Bookmarks](./demo/bookmarks-view.png)

### Search Input

![Search Input](./demo/search-input.png)

### Import Input

![Import Input](./demo/import-input.png)

## Features

### Reading

* Supports `TXT`
* Supports `EPUB`
* Automatically restores the last reading position
* Chapter TOC, previous chapter, next chapter
* Full-text search with `n / N` result navigation
* Highlights matches on the current page
* Save, list, delete, and jump to bookmarks
* Page scrolling and configurable visible text lines per page
* Adjustable content width, margins, top padding, line spacing, text color, high contrast mode, basic color mode, and auto-page interval
* Reflows text based on terminal width and wide-character display width

### Bookshelf

* Starts in bookshelf mode when launched without arguments
* Supports importing a single book or importing from a directory
* Directory import scans only the current level by default, with optional recursive mode
* Tracks format, chapter, progress, and last read time per book
* Sort by recent activity, import time, or title
* Filter by format and reading status
* Remove from bookshelf only, or remove and delete the local file
* Main bookshelf list focuses on titles, while the right panel shows details
* Uses EPUB metadata or extracted content title when available, otherwise falls back to filename
* Cleans up common source suffixes such as `Z-Library`

### UI and Interaction

* Three themes: `vscode`, `jetbrains`, `ops-console`
* Dedicated status and shortcut hints for bookshelf, reading, TOC, and bookmarks
* Import path input supports cursor movement, `Tab` completion, candidate selection, and paging
* Drag files or directories directly into the import input
* Supports both Vim-style keys and arrow-key navigation
* Automatically checks GitHub Releases for updates on startup and shows the current version
* Manual update check, release notes preview, and in-app self-update
* Boss Key support, including a custom external command
* Auto page turning

## Terminal Compatibility

ReadCLI does not depend on macOS or iTerm-specific layout behavior. It works with common Linux terminals too, for example:

* `gnome-terminal`
* `kitty`
* `wezterm`
* `alacritty`
* `xterm`
* `tmux` / `screen`

The main differences come from terminal capabilities:

* Color quality depends on whether the terminal supports `256 colors` or better
* Dragging files or directories into the input box depends on the terminal emulator itself

If custom colors look dim or off in some Linux terminals, you can force basic ANSI colors in `config.json`:

```json
{
  "force_basic_color": true
}
```

## Fork Notes

This repository is based on the original project [TimothyYe/glance](https://github.com/TimothyYe/glance).

The original license and base ideas are preserved, while this fork continues active maintenance and feature work.

If you want the original project, see:
[https://github.com/TimothyYe/glance](https://github.com/TimothyYe/glance)

## Download and Install

Prebuilt binaries:

* [Releases](https://github.com/lvshp/ReadCLI/releases)

Currently provided:

* macOS arm64
* macOS amd64
* Linux amd64

Build from source:

```bash
git clone https://github.com/lvshp/ReadCLI.git
cd ReadCLI
go build -o readcli ./cmd
```

If the binary is already in your `PATH`, just run:

```bash
readcli
```

To use `readcli` globally, place it in `~/.local/bin` and make sure that directory is in `PATH`:

```bash
mkdir -p ~/.local/bin
cp ./readcli ~/.local/bin/readcli
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

Then verify:

```bash
command -v readcli
```

Expected output:

```bash
/Users/your-name/.local/bin/readcli
```

Or open a book directly:

```bash
readcli /path/to/book.epub
readcli /path/to/book.txt
```

## Quick Start

### 1. Open the bookshelf

```bash
readcli
```

Use `i` to import books, `↑/↓` or `j/k` to move, and `→` or `Enter` to open.

### 2. Open a book directly

```bash
readcli -n 8 /path/to/book.epub
```

The book will be added to the bookshelf automatically, progress will be restored, and the reading page will start with 8 visible content lines per page.

### 3. Import files

The import input supports path completion, candidate selection, drag-and-drop, and `Ctrl-r` to toggle between current-directory-only and recursive import.

### 4. Adjust reading style

Press `,` while reading to open the reading settings panel. You can adjust:

* content width
* left / right / top / bottom margins
* line spacing
* text color
* high contrast mode
* basic color mode
* auto-page interval

Press `c` to cycle through bright preset colors.
Press `t` to toggle auto page turning.

### 5. Check for updates

ReadCLI checks for updates on startup.

If you skip an update from the automatic prompt, that version will not be shown again automatically. You can still press `u` at any time to run a manual update check.

When you confirm an update, ReadCLI downloads the correct release package for the current platform and replaces the current binary. Restart the app after the update finishes.

### 6. Configure Boss Key

By default, pressing `b` switches to the built-in disguise page.

If you want the Boss Key to run an external command instead, set `boss_key_command` in `config.json`:

```json
{
  "boss_key_command": "genact"
}
```

You can also provide a full command with arguments:

```json
{
  "boss_key_command": "/usr/local/bin/genact -m terraform"
}
```

After that, pressing `b` temporarily leaves the ReadCLI UI, runs the configured command in the current terminal, and returns to ReadCLI when that command exits.

Recommended project:
[svenstaro/genact](https://github.com/svenstaro/genact)

## Keybindings

Press `?` to open the built-in help page. Both Vim-style keys and arrow keys are supported.

### Bookshelf

* Vim-style: `j/k` move, `Enter` open, `i` import, `o/r` sort and filter, `x` remove, `u` check updates
* Arrow keys: `↑/↓` move, `→` or `Enter` open, `u` check updates

### Reading

* Vim-style: `j/k` page down/up, `[` / `]` previous/next chapter, `/` search, `n/N` next/previous result, `s/B` bookmarks, `m` TOC, `c` text color, `u` check updates
* Arrow keys: `↑/↓` page down/up, `←/→` previous/next chapter, `u` check updates
* Reading settings: press `,` to adjust content width, margins, line spacing, text color, high contrast mode, basic color mode, and auto-page interval

### TOC / Bookmarks

* TOC supports direct chapter jumping by number, and the TOC titles are displayed in Chinese without the old left-side numbering
* Vim-style: `j/k` move, `Enter` open
* Arrow keys: `↑/↓` move, `→` or `Enter` open, `←` return

### General

* `+ / -` adjust visible content lines per page
* `c` cycle text color
* `T` switch theme
* `u` manually check for updates
* `p` show reading progress
* `b` trigger Boss Key
* `f` show or hide borders
* `q` return to bookshelf or quit

## Data Files

Local data is stored by default in:

```bash
~/.readcli/
```

This directory contains:

* `config.json`
* `bookshelf.json`
* `bookmarks.json`
* `progress.json`

`config.json` stores reading-related settings such as:

* content width ratio
* margins
* Boss Key custom command
* line spacing
* auto-page interval in milliseconds
* text color (`#RRGGBB`, `#RGB`, `R,G,B`)
* `reading_high_contrast`
* `force_basic_color`

## Development

Issues and pull requests are welcome.

Related documents:

* [CHANGELOG](./CHANGELOG.md)
* [CONTRIBUTING](./CONTRIBUTING.md)

## Links

* [linux.do](https://linux.do/)

## License

This project continues to use [Apache License 2.0](./LICENSE).
