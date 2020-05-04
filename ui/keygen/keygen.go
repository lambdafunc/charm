package keygen

import (
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/charm"
	"github.com/charmbracelet/charm/ui/common"
	"github.com/charmbracelet/tea"
	"github.com/charmbracelet/teaparty/spinner"
	"github.com/muesli/reflow/indent"
	"github.com/muesli/termenv"
)

type status int

const (
	statusRunning status = iota
	statusError
	statusSuccess
	statusDone
	statusQuitting
)

// MSG

type failedMsg error

type successMsg struct{}

type DoneMsg struct{}

// MODEL

type Model struct {
	status     status
	err        error
	spinner    spinner.Model
	standalone bool
}

// INIT

func Init() (tea.Model, tea.Cmd) {
	m := NewModel()
	m.standalone = true
	return m, GenerateKeys
}

func NewModel() Model {
	s := spinner.NewModel()
	s.Type = spinner.Dot
	s.ForegroundColor = common.SpinnerColor
	return Model{
		status:     statusRunning,
		err:        nil,
		spinner:    s,
		standalone: false,
	}
}

// UPDATE

func Update(msg tea.Msg, model tea.Model) (tea.Model, tea.Cmd) {
	m, ok := model.(Model)
	if !ok {
		return model, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			fallthrough
		case "esc":
			fallthrough
		case "q":
			m.status = statusQuitting
			return m, tea.Quit
		}
	case failedMsg:
		m.err = msg
		m.status = statusError
		return m, tea.Quit
	case successMsg:
		m.status = statusSuccess
		return m, pause
	case spinner.TickMsg:
		m.spinner, _ = spinner.Update(msg, m.spinner)
		return m, nil
	case DoneMsg:
		if m.standalone {
			return m, tea.Quit
		}
		m.status = statusDone
		return m, nil
	}

	return m, nil
}

// VIEWS

func View(model tea.Model) string {
	m, ok := model.(Model)
	if !ok {
		return "could not perform assertion on model in view"
	}

	var s string

	switch m.status {
	case statusRunning:
		s += fmt.Sprintf("%s Generating keys...", spinner.View(m.spinner))
	case statusSuccess:
		s += termenv.String("✔").Foreground(common.Green.Color()).String()
		s += "  Done!"
	case statusError:
		s += fmt.Sprintf("Uh oh, there's been an error: %v", m.err)
	case statusQuitting:
		s += "Exiting..."
	}

	if m.standalone {
		return indent.String(fmt.Sprintf("\n%s\n", s), 2)
	}

	return s
}

// SUBSCRIPTIONS

func Subscriptions(model tea.Model) tea.Subs {
	m, ok := model.(Model)
	if !ok {
		return nil
	}
	sub, err := Spin(m)
	if err != nil {
		return nil
	}

	if m.status == statusRunning {
		return tea.Subs{"keygen-spinner": sub}
	}
	return nil
}

func Spin(model tea.Model) (tea.Sub, error) {
	m, ok := model.(Model)
	if !ok {
		return nil, errors.New("could not perform assertion on model")
	}
	sub, err := spinner.MakeSub(m.spinner)
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// COMMANDS

// GenerateKeys is a Tea command that generates a pair of SSH keys and writes
// them to disk
func GenerateKeys() tea.Msg {
	_, err := charm.NewSSHKeyPair()
	if err != nil {
		return failedMsg(err)
	}
	return successMsg{}
}

// pause runs the final pause before we wrap things up
func pause() tea.Msg {
	time.Sleep(time.Millisecond * 500)
	return DoneMsg{}
}
