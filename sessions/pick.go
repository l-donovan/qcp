package sessions

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/l-donovan/qcp/common"
	"golang.org/x/crypto/ssh"
)

var (
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFDF5")).
			Background(lipgloss.Color("#25A065")).
			Padding(0, 1)
)

func newItemDelegate(keys *delegateKeyMap, app *pickSession) list.ItemDelegate {
	d := list.NewDefaultDelegate()

	d.UpdateFunc = func(msg tea.Msg, m *list.Model) tea.Cmd {
		entry, ok := m.SelectedItem().(common.ThinDirEntry)

		if !ok {
			return nil
		}

		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch {
			case key.Matches(msg, keys.choose):
				if err := app.SelectFile(entry); err != nil {
					return m.NewStatusMessage(fmt.Sprintf("Failed to select %s: %v", entry.Name, err))
				}
			case key.Matches(msg, keys.enter):
				if !entry.Mode.IsDir() {
					return m.NewStatusMessage(fmt.Sprintf("Can't enter non-directory %s", entry.Name))
				}

				if err := app.EnterDirectory(entry.Name); err != nil {
					return m.NewStatusMessage(fmt.Sprintf("Failed to enter directory %s: %v", entry.Name, err))
				}

				entries, err := app.GetFiles()

				if err != nil {
					return m.NewStatusMessage(fmt.Sprintf("Failed to list files: %v", err))
				}

				cmd := m.SetItems(entries)
				app.Update(cmd)
			case key.Matches(msg, keys.up):
				// Whether this is mutating or not is irrelevant as we're about to replace entries entirely
				if err := app.EnterDirectory(".."); err != nil {
					return m.NewStatusMessage(fmt.Sprintf("Failed to go up a level: %v", err))
				}

				entries, err := app.GetFiles()

				if err != nil {
					return m.NewStatusMessage(fmt.Sprintf("Failed to list files: %v", err))
				}

				cmd := m.SetItems(entries)
				app.Update(cmd)
			}
		}

		return nil
	}

	help := []key.Binding{keys.choose, keys.enter, keys.up}

	d.ShortHelpFunc = func() []key.Binding {
		return help
	}

	d.FullHelpFunc = func() [][]key.Binding {
		return [][]key.Binding{help}
	}

	return d
}

type delegateKeyMap struct {
	choose key.Binding
	enter  key.Binding
	up     key.Binding
}

func (d delegateKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		d.choose,
		d.enter,
		d.up,
	}
}

func (d delegateKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		d.ShortHelp(),
	}
}

func newDelegateKeyMap() *delegateKeyMap {
	return &delegateKeyMap{
		choose: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "choose"),
		),
		enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "enter"),
		),
		up: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "up a level"),
		),
	}
}

type listKeyMap struct {
	toggleSpinner    key.Binding
	toggleTitleBar   key.Binding
	toggleStatusBar  key.Binding
	togglePagination key.Binding
	toggleHelpMenu   key.Binding
}

func newListKeyMap() *listKeyMap {
	return &listKeyMap{
		toggleSpinner: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "toggle spinner"),
		),
		toggleTitleBar: key.NewBinding(
			key.WithKeys("T"),
			key.WithHelp("T", "toggle title"),
		),
		toggleStatusBar: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "toggle status"),
		),
		togglePagination: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "toggle pagination"),
		),
		toggleHelpMenu: key.NewBinding(
			key.WithKeys("H"),
			key.WithHelp("H", "toggle help"),
		),
	}
}

type pickSession struct {
	path          *string
	list          list.Model
	keys          *listKeyMap
	delegateKeys  *delegateKeyMap
	browseSession BrowseSession
}

func newPickSession(session BrowseSession, location string) (pickSession, error) {
	var (
		delegateKeys = newDelegateKeyMap()
		listKeys     = newListKeyMap()
	)

	m := pickSession{
		path:          &location,
		browseSession: session,
		delegateKeys:  delegateKeys,
		keys:          listKeys,
	}

	entries, err := m.GetFiles()

	if err != nil {
		return m, err
	}

	// Setup list
	delegate := newItemDelegate(delegateKeys, &m)
	itemList := list.New(entries, delegate, 0, 0)
	itemList.Title = fmt.Sprintf("[%s]", *m.path)
	itemList.Styles.Title = titleStyle
	itemList.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			listKeys.toggleSpinner,
			listKeys.toggleTitleBar,
			listKeys.toggleStatusBar,
			listKeys.togglePagination,
			listKeys.toggleHelpMenu,
		}
	}

	m.list = itemList

	return m, nil
}

func (m pickSession) Init() tea.Cmd {
	return nil
}

func (m pickSession) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	case tea.KeyMsg:
		// Don't match any of the keys below if we're actively filtering.
		if m.list.FilterState() == list.Filtering {
			break
		}

		switch {
		case key.Matches(msg, m.keys.toggleSpinner):
			cmd := m.list.ToggleSpinner()
			return m, cmd
		case key.Matches(msg, m.keys.toggleTitleBar):
			v := !m.list.ShowTitle()
			m.list.SetShowTitle(v)
			m.list.SetShowFilter(v)
			m.list.SetFilteringEnabled(v)
			return m, nil
		case key.Matches(msg, m.keys.toggleStatusBar):
			m.list.SetShowStatusBar(!m.list.ShowStatusBar())
			return m, nil
		case key.Matches(msg, m.keys.togglePagination):
			m.list.SetShowPagination(!m.list.ShowPagination())
			return m, nil
		case key.Matches(msg, m.keys.toggleHelpMenu):
			m.list.SetShowHelp(!m.list.ShowHelp())
			return m, nil
		}
	}

	// This will also call our delegate's update function.
	m.list.Title = fmt.Sprintf("[%s]", *m.path)
	newListModel, cmd := m.list.Update(msg)
	m.list = newListModel
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m pickSession) View() string {
	return appStyle.Render(m.list.View())
}

func (m pickSession) GetFiles() ([]list.Item, error) {
	contents, err := m.browseSession.ListContents()

	if err != nil {
		return nil, fmt.Errorf("list contents: %v", err)
	}

	entries := make([]list.Item, len(contents))

	for i, item := range contents {
		entries[i] = item
	}

	return entries, nil
}

func (m pickSession) SelectFile(entry common.ThinDirEntry) error {
	// TODO: Parameterize `compress`.
	downloadSession, err := m.browseSession.DownloadFile(entry.Name, true)

	if err != nil {
		return fmt.Errorf("start download %s: %v", entry.Name, err)
	}

	defer downloadSession.Stop()

	downloadInfo, err := downloadSession.GetDownloadInfo(entry.Name)

	if err != nil {
		return fmt.Errorf("select file: %v", err)
	}

	if err := downloadInfo.Receive(); err != nil {
		return fmt.Errorf("receive %s: %v", entry.Name, err)
	}

	return nil
}

func (m *pickSession) EnterDirectory(location string) error {
	return m.browseSession.EnterDirectory(location)
}

func Pick(client *ssh.Client, location string) error {
	browseSession, err := Browse(client, location)

	if err != nil {
		return fmt.Errorf("present %s: %v", location, err)
	}

	defer browseSession.Stop()

	pickSession, err := newPickSession(browseSession, location)

	if err != nil {
		return err
	}

	if _, err := tea.NewProgram(pickSession, tea.WithAltScreen()).Run(); err != nil {
		return err
	}

	return nil
}
