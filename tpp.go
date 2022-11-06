package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.design/x/clipboard"
)

const useHighPerformanceRenderer = false

var (
	errorStyle = func() lipgloss.Style {
		return lipgloss.NewStyle().Padding(1, 1)
	}()

	titleStyle = func() lipgloss.Style {
		b := lipgloss.NormalBorder()
		b.Right = "├"
		return lipgloss.NewStyle().BorderStyle(b).Padding(0, 1)
	}()

	infoStyle = func() lipgloss.Style {
		b := lipgloss.RoundedBorder()
		b.Left = "┤"
		return titleStyle.Copy().BorderStyle(b)
	}()
)

type PipePreview struct {
	ready bool

	stdin  string
	stderr string
	stdout string

	prev string
	pipe textinput.Model

	lastYOffset int
	preview     viewport.Model

	cmd    *exec.Cmd
	shell  string
	invoke string
}

func NewPipePreview(stdin string) *PipePreview {
	ti := textinput.New()
	ti.Focus()
	ti.Prompt = "| "
	ti.PromptStyle = ti.PromptStyle.Padding(0, 1)

	shell := "bash"
	if s := os.Getenv("SHELL"); s != "" {
		shell = s
	}

	invoke := "-c"
	// for future if shell has different invocation flag
	// exe := filepath.Base(shell)
	// switch exe {}

	return &PipePreview{
		ready:  false,
		stdin:  stdin,
		stdout: stdin,

		pipe:        ti,
		lastYOffset: 0,

		shell:  shell,
		invoke: invoke,
	}
}

func (self PipePreview) Flush() {
	fmt.Fprint(os.Stdout, self.stdout)
	fmt.Fprint(os.Stderr, self.stderr)
}

func (self *PipePreview) RunCmd() {
	value := self.pipe.Value()

	// clear
	if value == "" {
		self.stdout = self.stdin
		self.stderr = ""
		return
	}

	// ready cmd
	// TODO: get current shell decipher which flag to execute with
	// self.cmd = exec.Command("zsh", "-c", value)
	self.cmd = exec.Command(self.shell, self.invoke, value)

	// setup std in/out/err
	var stdout, stderr bytes.Buffer
	self.cmd.Stdout = &stdout
	self.cmd.Stderr = &stderr
	self.cmd.Stdin = bytes.NewReader([]byte(self.stdin))

	// run cmd
	if err := self.cmd.Run(); err != nil {
		self.stderr = err.Error()
		return
	}

	self.stdout, self.stderr = string(stdout.Bytes()), string(stderr.Bytes())
	self.preview.SetContent(self.stdout)
	self.cmd = nil
	return
}

func (self PipePreview) Init() tea.Cmd {
	return nil
}

func (self PipePreview) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd  tea.Cmd
		cmds []tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		k := msg.String()
		if k == "ctrl+o" {
			clipboard.Write(clipboard.FmtText, []byte(self.stdout))
		} else if k == "ctrl+p" {
			clipboard.Write(clipboard.FmtText, []byte(self.pipe.Value()))
		} else if k == "ctrl+c" || k == "ctrl+q" || k == "esc" {
			return self, tea.Quit
		} else if k == "tab" {
			if self.pipe.Focused() {
				self.pipe.Blur()
			} else {
				self.pipe.Focus()
			}
		}

	case tea.WindowSizeMsg:
		inputHeight := lipgloss.Height(self.pipe.View())
		errorHeight := lipgloss.Height(self.ErrorView())
		headerHeight := lipgloss.Height(self.HeaderView())
		footerHeight := lipgloss.Height(self.FooterView())
		verticalMarginHeight := inputHeight + errorHeight + headerHeight + footerHeight
		if !self.ready {
			// Since this program is using the full size of the viewport we
			// need to wait until we've received the window dimensions before
			// we can initialize the viewport. The initial dimensions come in
			// quickly, though asynchronously, which is why we wait for them
			// here.
			self.preview = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			self.preview.YPosition = headerHeight + inputHeight + errorHeight
			self.preview.HighPerformanceRendering = useHighPerformanceRenderer
			self.preview.SetContent(self.stdout)
			self.ready = true

			// This is only necessary for high performance rendering, which in
			// most cases you won't need.
			//
			// Render the viewport one line below the header.
			self.preview.YPosition = headerHeight + 1
		} else {
			self.preview.Width = msg.Width
			self.preview.Height = msg.Height - verticalMarginHeight
		}

		if useHighPerformanceRenderer {
			// Render (or re-render) the whole viewport. Necessary both to
			// initialize the viewport and when the window is resized.
			//
			// This is needed for high-performance rendering only.
			cmds = append(cmds, viewport.Sync(self.preview))
		}
	}

	// update pipe input
	current := self.pipe.Value()
	self.pipe, cmd = self.pipe.Update(msg)
	cmds = append(cmds, cmd)

	// freeze preview
	previewMessage := msg
	focused := self.pipe.Focused()
	if focused {
		previewMessage = nil
		self.preview.SetYOffset(self.lastYOffset)
	} else {
		self.lastYOffset = self.preview.YOffset
	}

	// set fainting
	self.pipe.TextStyle = self.pipe.TextStyle.Faint(!focused)
	self.pipe.PromptStyle = self.pipe.PromptStyle.Faint(!focused)
	self.preview.Style = self.preview.Style.Faint(focused)

	// update preview
	self.preview, cmd = self.preview.Update(previewMessage)
	cmds = append(cmds, cmd)

	// run command
	if current != self.prev {
		// TODO: Look into refactoring so this is threaded
		// if self.cmd != nil {
		// 	self.cmd.Process.Kill()
		// }

		self.RunCmd()
	}

	self.prev = current
	return self, tea.Batch(cmds...)
}

func (self PipePreview) View() string {
	if !self.ready {
		return "\n  Initializing..."
	}

	return fmt.Sprintf("%s\n%s\n%s\n%s\n%s",
		self.pipe.View(),
		self.ErrorView(),
		self.HeaderView(),
		self.preview.View(),
		self.FooterView())
}

func (self PipePreview) ErrorView() string {
	prefix := "Error: "
	if self.stderr == "" {
		prefix = ""
	}

	err := errorStyle.
		Faint(self.stderr == "").
		Width(self.preview.Width - lipgloss.Width(prefix)).
		Render(prefix + self.stderr)
	return err
}

func (self PipePreview) HeaderView() string {
	title := titleStyle.
		Faint(self.pipe.Focused()).
		Render("Output")
	line := strings.Repeat("─", max(0, self.preview.Width-lipgloss.Width(title)))
	return lipgloss.JoinHorizontal(lipgloss.Center, title, line)
}

func (self PipePreview) FooterView() string {
	info := infoStyle.
		Faint(self.pipe.Focused()).
		Render(fmt.Sprintf("%3.f%%", self.preview.ScrollPercent()*100))
	line := strings.Repeat("─", max(0, self.preview.Width-lipgloss.Width(info)))
	return lipgloss.JoinHorizontal(lipgloss.Center, line, info)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func help() {

}

func main() {
	help := flag.Bool("h", false, "print help information")

	flag.Usage = func() {
		flagSet := flag.CommandLine
		fmt.Printf("pipe preview requires input piped from standard in.\n")
		fmt.Printf("stderr and stdout flushed from preview are flushed upon exit.\n")
		fmt.Printf("flags:\n")
		order := []string{"h"}
		for _, name := range order {
			flag := flagSet.Lookup(name)
			fmt.Printf("-%s\t%s\n", flag.Name, flag.Usage)
		}
		fmt.Printf("keybinds:\n")
		fmt.Printf("- %s\t%s\n", "tab", "swap between and input and preview")
		fmt.Printf("- %s\t%s\n", "ctrl+p", "copy input to clipboard")
		fmt.Printf("- %s\t%s\n", "ctrl+o", "copy preview to clipboard")
		fmt.Printf("- %s\t%s\n", "esc/ctrl+q/ctrl+c", "exit")
	}

	flag.Parse()

	if *help {
		flag.Usage()
		return
	}

	info, err := os.Stdin.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		flag.Usage()
		return
	}

	if info.Mode()&os.ModeCharDevice != 0 {
		fmt.Fprintln(os.Stderr, errors.New("not a valid pipe"))
		flag.Usage()
		return
	}

	reader := bufio.NewReader(os.Stdin)
	var output []rune

	for {
		input, _, err := reader.ReadRune()
		if err != nil && err == io.EOF {
			break
		}
		output = append(output, input)
	}

	p := tea.NewProgram(
		NewPipePreview(string(output)),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	model, err := p.StartReturningModel()
	if err != nil {
		panic(err)
	}

	// flush backout stdout and stderr
	model.(PipePreview).Flush()
}
