# Pipe Preview

Preview piped input and execute commands on it. Inspired by https://github.com/akavel/up and https://github.com/fiatjaf/jiq with a TUI built using https://github.com/charmbracelet/bubbletea.

Expects a `SHELL` variable in your environment, will default to `bash` if not set. Currently only supports shells that invoke string based commands via a `-c` flag.

Upon exit, stderr and stdout from most recent execution are flushed back out to your terminal.

![pipe preview](https://github.com/arpachuilo/pipe-preview/blob/main/demo.gif?raw=true)

## Installation

### Via `go install`

```bash
go install github.com/arpachuilo/pipe-preview@latest
```

## Keybinds

- `tab` swap between input and preview
- `ctrl+p` copy input to clipboard
- `ctrl+o` copy preview to clipboard
- `esc` / `ctrl+q` / `ctrl+c` exit
