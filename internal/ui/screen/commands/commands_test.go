package commands

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/mcnull/sshex/internal/ui/screen"
	"github.com/mcnull/sshex/internal/ui/theme"
)

type fakeSource struct {
	snapshot Snapshot
	err      error
	calls    int
}

func (s *fakeSource) List() (Snapshot, error) {
	s.calls++
	return s.snapshot, s.err
}

type updateCall struct {
	name    string
	command Command
}

type toggleCall struct {
	name     string
	disabled bool
}

type fakeController struct {
	addCalls    []Command
	updateCalls []updateCall
	removeCalls []string
	toggleCalls []toggleCall
	addErr      error
	updateErr   error
	removeErr   error
	toggleErr   error
}

func (c *fakeController) Add(command Command) error {
	c.addCalls = append(c.addCalls, command)
	return c.addErr
}

func (c *fakeController) Update(name string, command Command) error {
	c.updateCalls = append(c.updateCalls, updateCall{name: name, command: command})
	return c.updateErr
}

func (c *fakeController) Remove(name string) error {
	c.removeCalls = append(c.removeCalls, name)
	return c.removeErr
}

func (c *fakeController) SetDisabled(name string, disabled bool) error {
	c.toggleCalls = append(c.toggleCalls, toggleCall{name: name, disabled: disabled})
	return c.toggleErr
}

func newScreen(source Source, controller Controller) *Model {
	styles := theme.NewStyles(theme.Load(theme.Options{NoColor: true}))
	m := New(Options{
		Styles:     &styles,
		Source:     source,
		Controller: controller,
	})
	m.Resize(60, 12)
	return m
}

func editableSource(items ...Command) *fakeSource {
	return &fakeSource{snapshot: Snapshot{Commands: items, Editable: true}}
}

func testCommands() []Command {
	return []Command{
		{Name: "code", Template: `code --folder-uri "vscode-remote://ssh-remote+${REMOTE_HOST}"`, Alias: "code"},
		{Name: "git-status", Template: "git status --short", Alias: "gs", Disabled: true},
	}
}

func load(t *testing.T, m *Model) {
	t.Helper()
	cmd := m.load(false)
	if cmd == nil {
		t.Fatal("load returned no command")
	}
	m.Update(cmd())
}

func press(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

func typeText(m *Model, text string) {
	for _, r := range text {
		m.Update(press(r, string(r)))
	}
}

func action(t *testing.T, cmd tea.Cmd) screen.ActionMsg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msg, ok := cmd().(screen.ActionMsg)
	if !ok {
		t.Fatalf("message = %T, want screen.ActionMsg", cmd())
	}
	return msg
}

func firstBatch(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command, got nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return msg
	}
	if len(batch) == 0 {
		t.Fatal("empty batch")
	}
	return batch[0]()
}

func listFocus(m *Model) {
	m.setFocus(focusList)
}

func searchFocus(m *Model) {
	m.setFocus(focusSearch)
}

func commandNames(items []Command) []string {
	names := make([]string, len(items))
	for i, item := range items {
		names[i] = item.Name
	}
	return names
}

func TestLoadingAndEmptyStates(t *testing.T) {
	m := newScreen(editableSource(), &fakeController{})
	if !strings.Contains(ansi.Strip(m.View()), "loading commands…") {
		t.Fatalf("loading state is missing:\n%s", m.View())
	}

	load(t, m)
	if !strings.Contains(ansi.Strip(m.View()), "no commands") {
		t.Fatalf("empty state is missing:\n%s", m.View())
	}
}

func TestListIsFocusedByDefault(t *testing.T) {
	m := newScreen(editableSource(testCommands()...), &fakeController{})
	load(t, m)

	if m.focus != focusList {
		t.Fatalf("focus = %d, want the list", m.focus)
	}
	hasAdd := false
	hasSearch := false
	for _, binding := range m.Keys() {
		switch binding.Help().Key {
		case "a":
			hasAdd = binding.Help().Desc == "add"
		case "/":
			hasSearch = binding.Help().Desc == "search"
		}
	}
	if !hasAdd {
		t.Fatal("mutation keys are hidden until search is focused")
	}
	if !hasSearch {
		t.Fatal("the / search binding is missing")
	}

	// Backspace no longer leaves the list or edits the query.
	m.Update(press(tea.KeyBackspace, ""))
	if m.focus != focusList || m.query != "" {
		t.Fatalf("backspace changed focus or query: focus = %d, query = %q", m.focus, m.query)
	}

	// Printable characters only filter once the search field is focused.
	typeText(m, "zz")
	if len(m.filtered) != 2 || m.query != "" {
		t.Fatalf("list focus consumed text: query = %q, filtered = %v", m.query, commandNames(m.filtered))
	}
	m.Update(press('/', "/"))
	if m.focus != focusSearch {
		t.Fatalf("/ focus = %d, want the search field", m.focus)
	}
	typeText(m, "git")
	if got := fmt.Sprint(commandNames(m.filtered)); got != "[git-status]" {
		t.Fatalf("filtered = %v, want [git-status]", got)
	}
}

func commandHelp(m *Model, keyName string) string {
	for _, binding := range m.Keys() {
		if binding.Help().Key == keyName {
			return binding.Help().Desc
		}
	}
	return ""
}

func TestFooterHelpIsCompactAndStateful(t *testing.T) {
	m := newScreen(editableSource(testCommands()...), &fakeController{})
	load(t, m)

	if got := commandHelp(m, "↑/k/↓/j"); got != "navigate" {
		t.Fatalf("navigate help = %q", got)
	}
	if got := commandHelp(m, "a"); got != "add" {
		t.Fatalf("add help = %q", got)
	}
	if got := commandHelp(m, "x"); got != "remove" {
		t.Fatalf("remove help = %q", got)
	}
	if got := commandHelp(m, "space"); got != "disable" {
		t.Fatalf("toggle help = %q, want disable for an enabled command", got)
	}

	m.Update(press(tea.KeyDown, ""))
	if got := commandHelp(m, "space"); got != "enable" {
		t.Fatalf("toggle help = %q, want enable for a disabled command", got)
	}
}

func TestLoadsAndRendersCommands(t *testing.T) {
	m := newScreen(editableSource(testCommands()...), &fakeController{})
	load(t, m)

	view := ansi.Strip(m.View())
	for _, want := range []string{
		"NAME", "ALIAS", "STATE",
		"code", "git-status", "gs", "enabled", "disabled",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("view is missing %q:\n%s", want, view)
		}
	}
	lines := strings.Split(view, "\n")
	if last := lines[len(lines)-1]; strings.TrimSpace(last) != "" {
		t.Fatalf("last line = %q, want empty", last)
	}
	for _, unwanted := range []string{"code --folder-uri", "git status --short", "alias "} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("view should not contain %q:\n%s", unwanted, view)
		}
	}
}

func TestReadOnlyStatus(t *testing.T) {
	m := newScreen(&fakeSource{snapshot: Snapshot{Commands: testCommands()}}, &fakeController{})
	load(t, m)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "read-only") || !strings.Contains(view, "managed on the origin") {
		t.Fatalf("read-only status is missing:\n%s", view)
	}
}

func TestSearchPromptAlignsWithNameColumnAndMarksFocus(t *testing.T) {
	m := newScreen(editableSource(testCommands()...), &fakeController{})
	load(t, m)

	lines := func() []string {
		return strings.Split(ansi.Strip(m.View()), "\n")
	}
	unfocused := lines()
	nameColumn := strings.Index(unfocused[3], "NAME")
	if got := strings.Index(unfocused[1], "search"); got != nameColumn {
		t.Fatalf("search starts at %d, NAME at %d:\n%s", got, nameColumn, unfocused[1])
	}
	if strings.Contains(unfocused[1], ">") {
		t.Fatalf("search line = %q, want no focus marker", unfocused[1])
	}

	searchFocus(m)
	focused := lines()
	if !strings.Contains(focused[1], "> search") {
		t.Fatalf("search line = %q, want focus marker", focused[1])
	}
	if got := strings.Index(focused[1], "search"); got != nameColumn {
		t.Fatalf("focused search starts at %d, NAME at %d:\n%s", got, nameColumn, focused[1])
	}

	m.input.Prompt = m.searchPrompt()
	m.input.SetStyles(inputStyles(m.styles))
	if got := m.input.Styles().Blurred.Prompt.GetForeground(); fmt.Sprint(got) != fmt.Sprint(m.styles.Muted.GetForeground()) {
		t.Fatalf("blurred prompt foreground = %v, want muted", got)
	}
}

func TestSortsByName(t *testing.T) {
	source := editableSource(
		Command{Name: "zed", Template: "zed"},
		Command{Name: "alpha", Template: "alpha"},
		Command{Name: "Beta", Template: "beta"},
	)
	m := newScreen(source, &fakeController{})
	load(t, m)

	if got := fmt.Sprint(commandNames(m.commands)); got != "[alpha Beta zed]" {
		t.Fatalf("order = %v, want [alpha Beta zed]", got)
	}

	// A source returning a different order must not reorder the rows.
	source.snapshot.Commands = []Command{
		Command{Name: "Beta", Template: "beta"},
		Command{Name: "zed", Template: "zed"},
		Command{Name: "alpha", Template: "alpha"},
	}
	load(t, m)
	if got := fmt.Sprint(commandNames(m.commands)); got != "[alpha Beta zed]" {
		t.Fatalf("order after refresh = %v, want [alpha Beta zed]", got)
	}
}

func TestSelectionSurvivesRefresh(t *testing.T) {
	source := editableSource(testCommands()...)
	m := newScreen(source, &fakeController{})
	load(t, m)
	listFocus(m)
	m.Update(press(tea.KeyDown, ""))

	source.snapshot.Commands = []Command{
		{Name: "git-status", Template: "git status --short"},
		{Name: "code", Template: "code"},
	}
	load(t, m)

	if got := m.selectedName(); got != "git-status" {
		t.Fatalf("selected = %q, want git-status", got)
	}
}

func TestSearchFiltersByNameAndAlias(t *testing.T) {
	source := editableSource(
		Command{Name: "code", Template: "code", Alias: "code"},
		Command{Name: "git-status", Template: "git status", Alias: "gs"},
		Command{Name: "zed", Template: "zed"},
	)
	m := newScreen(source, &fakeController{})
	load(t, m)
	searchFocus(m)

	typeText(m, "gs")
	if got := fmt.Sprint(commandNames(m.filtered)); got != "[git-status]" {
		t.Fatalf("alias match = %v, want [git-status]", got)
	}
	if view := ansi.Strip(m.View()); strings.Contains(view, "of 3 commands") {
		t.Fatalf("filtered count should be hidden:\n%s", view)
	}

	m.Update(press(tea.KeyBackspace, ""))
	m.Update(press(tea.KeyBackspace, ""))
	typeText(m, "zed")
	if got := fmt.Sprint(commandNames(m.filtered)); got != "[zed]" {
		t.Fatalf("name match = %v, want [zed]", got)
	}
}

func TestSearchInputRefreshesStylesWhenThemeChanges(t *testing.T) {
	styles := theme.NewStyles(theme.Load(theme.Options{HasDarkBackground: true}))
	m := New(Options{Styles: &styles})
	m.Resize(60, 12)

	styles = theme.NewStyles(theme.Load(theme.Options{HasDarkBackground: false}))
	m.View()

	got := m.input.Styles().Focused
	if fmt.Sprint(got.Text.GetForeground()) != fmt.Sprint(styles.InputText.GetForeground()) {
		t.Fatalf("search text foreground = %v, want %v", got.Text.GetForeground(), styles.InputText.GetForeground())
	}
	if fmt.Sprint(got.Prompt.GetForeground()) != fmt.Sprint(styles.InputPrompt.GetForeground()) {
		t.Fatalf("search prompt foreground = %v, want %v", got.Prompt.GetForeground(), styles.InputPrompt.GetForeground())
	}
}

func TestNoMatchingCommands(t *testing.T) {
	m := newScreen(editableSource(testCommands()...), &fakeController{})
	load(t, m)
	searchFocus(m)

	typeText(m, "nope")
	if !strings.Contains(ansi.Strip(m.View()), "no matching commands") {
		t.Fatalf("no-match state is missing:\n%s", m.View())
	}
}

func TestLoadErrorIsShown(t *testing.T) {
	m := newScreen(&fakeSource{err: errors.New("boom")}, &fakeController{})
	load(t, m)

	if !strings.Contains(ansi.Strip(m.View()), "could not load commands: boom") {
		t.Fatalf("load error is missing:\n%s", m.View())
	}
	if m.mutationsAvailable() {
		t.Fatal("mutations are available after a failed initial load")
	}
}

func TestRefreshErrorKeepsExistingRows(t *testing.T) {
	source := editableSource(testCommands()...)
	m := newScreen(source, &fakeController{})
	load(t, m)

	source.err = errors.New("boom")
	load(t, m)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "git-status") {
		t.Fatal("existing commands disappeared after a refresh error")
	}
	if !strings.Contains(view, "could not refresh commands: boom") {
		t.Fatalf("refresh error is missing:\n%s", view)
	}
}

func TestTickRefreshesSilently(t *testing.T) {
	source := editableSource(testCommands()...)
	m := newScreen(source, &fakeController{})
	load(t, m)
	source.calls = 0

	_, cmd := m.Update(tickMsg{epoch: m.epoch})
	msg := firstBatch(t, cmd)
	m.Update(msg)

	if source.calls != 1 {
		t.Fatalf("source listed %d times, want 1", source.calls)
	}
	if !strings.Contains(ansi.Strip(m.View()), "git-status") {
		t.Fatal("list disappeared during a background refresh")
	}
}

func TestStalePollTickIsIgnored(t *testing.T) {
	source := editableSource(testCommands()...)
	m := newScreen(source, &fakeController{})
	m.Update(firstBatch(t, m.Init()))
	source.calls = 0

	_, cmd := m.Update(tickMsg{epoch: m.epoch - 1})
	if cmd != nil {
		t.Fatalf("stale tick returned a command (%T), want nil", cmd)
	}
	if source.calls != 0 {
		t.Fatalf("stale tick listed %d times, want 0", source.calls)
	}
}

func TestInitRecoversFromLostLoad(t *testing.T) {
	source := editableSource(testCommands()...)
	m := newScreen(source, &fakeController{})

	// Simulate a request whose result was delivered while another screen was
	// active and therefore never reached this model.
	m.load(false)

	m.Update(firstBatch(t, m.Init()))
	if !strings.Contains(ansi.Strip(m.View()), "git-status") {
		t.Fatalf("re-entering the screen did not reload the list:\n%s", m.View())
	}
}

func TestStaleLoadResultIsIgnored(t *testing.T) {
	source := editableSource(testCommands()...)
	m := newScreen(source, &fakeController{})
	m.inflight = false
	cmd := m.load(false)
	staleEpoch := m.loadEpoch - 1

	m.Update(loadedMsg{snapshot: Snapshot{Commands: []Command{{Name: "stale"}}, Editable: true}, epoch: staleEpoch})
	if len(m.commands) != 0 {
		t.Fatalf("stale result was applied: %+v", m.commands)
	}

	m.Update(cmd())
	if len(m.commands) != 2 {
		t.Fatalf("current result was discarded: %+v", m.commands)
	}
}

func TestInitReloadsAndResumeRefreshes(t *testing.T) {
	source := editableSource(testCommands()...)
	m := newScreen(source, &fakeController{})

	m.Update(firstBatch(t, m.Init()))
	if source.calls != 1 {
		t.Fatalf("source listed %d times, want 1", source.calls)
	}

	m.Update(firstBatch(t, m.Resume()))
	if source.calls != 2 {
		t.Fatalf("source listed %d times, want 2", source.calls)
	}
}

func TestEscPopsScreen(t *testing.T) {
	m := newScreen(editableSource(), &fakeController{})

	_, cmd := m.Update(press(tea.KeyEsc, ""))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPop {
		t.Fatalf("action = %v, want pop", msg.Action)
	}
}

func TestEditableOpensAddAndEdit(t *testing.T) {
	controller := &fakeController{}
	m := newScreen(editableSource(testCommands()...), controller)
	load(t, m)
	listFocus(m)

	var addedExisting []Command
	m.addOpener = func(existing []Command) screen.Screen {
		addedExisting = existing
		return NewForm(FormOptions{Styles: m.styles, Controller: controller})
	}
	_, cmd := m.Update(press('a', "a"))
	msg := action(t, cmd)
	if msg.Action != screen.ActionPush || msg.Screen == nil {
		t.Fatalf("action = %+v, want push", msg)
	}
	if got := msg.Screen.Title(); got != "Add command" {
		t.Fatalf("screen title = %q, want Add command", got)
	}
	if len(addedExisting) != 2 {
		t.Fatalf("add opener received %d commands, want 2", len(addedExisting))
	}

	var editedOriginal Command
	m.editOpener = func(existing []Command, original Command) screen.Screen {
		editedOriginal = original
		return NewForm(FormOptions{Styles: m.styles, Controller: controller, Original: &original})
	}
	_, cmd = m.Update(press(tea.KeyEnter, ""))
	msg = action(t, cmd)
	if msg.Action != screen.ActionPush || msg.Screen == nil {
		t.Fatalf("action = %+v, want push", msg)
	}
	if got := msg.Screen.Title(); got != "Edit command" {
		t.Fatalf("screen title = %q, want Edit command", got)
	}
	if editedOriginal.Name != "code" {
		t.Fatalf("edit opener received %q, want code", editedOriginal.Name)
	}
}

func TestReadOnlyHidesAndIgnoresMutations(t *testing.T) {
	m := newScreen(&fakeSource{snapshot: Snapshot{Commands: testCommands()}}, &fakeController{})
	load(t, m)
	listFocus(m)

	for _, binding := range m.Keys() {
		switch binding.Help().Key {
		case "a", "x", "space", "enter":
			t.Fatalf("mutation key %q is shown in read-only mode", binding.Help().Key)
		}
	}

	opened := false
	m.addOpener = func([]Command) screen.Screen {
		opened = true
		return NewForm(FormOptions{Styles: m.styles, Controller: &fakeController{}})
	}
	m.editOpener = func([]Command, Command) screen.Screen {
		opened = true
		return NewForm(FormOptions{Styles: m.styles, Controller: &fakeController{}})
	}
	for _, key := range []tea.KeyPressMsg{
		press('a', "a"),
		press('x', "x"),
		press(tea.KeyEnter, ""),
		{Code: tea.KeySpace, Text: " "},
	} {
		if _, cmd := m.Update(key); cmd != nil {
			t.Fatalf("read-only key %q returned a command (%T)", key.String(), cmd)
		}
	}
	if opened {
		t.Fatal("read-only mode opened a form")
	}
	if m.confirming {
		t.Fatal("read-only mode started a removal")
	}
}

func TestRemoveRequiresConfirmation(t *testing.T) {
	controller := &fakeController{}
	m := newScreen(editableSource(testCommands()...), controller)
	load(t, m)
	listFocus(m)

	m.Update(press('x', "x"))
	if !m.confirming {
		t.Fatal("x did not start a confirmation")
	}
	if len(controller.removeCalls) != 0 {
		t.Fatalf("remove called before confirmation: %v", controller.removeCalls)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "remove code?") {
		t.Fatalf("confirmation prompt is missing:\n%s", view)
	}
	if !strings.Contains(view, "new shells lose") {
		t.Fatalf("alias warning is missing:\n%s", view)
	}

	_, cmd := m.Update(press(tea.KeyEnter, ""))
	if cmd == nil {
		t.Fatal("confirm returned no command")
	}
	msg, ok := cmd().(removedMsg)
	if !ok {
		t.Fatalf("message = %T, want removedMsg", cmd())
	}
	m.Update(msg)

	if len(controller.removeCalls) != 1 || controller.removeCalls[0] != "code" {
		t.Fatalf("remove calls = %v, want [code]", controller.removeCalls)
	}
	if m.hasCommand("code") {
		t.Fatal("removed command is still in the list")
	}
	if !strings.Contains(ansi.Strip(m.View()), "command removed.") {
		t.Fatalf("remove status is missing:\n%s", m.View())
	}
}

func TestCancelRemoveKeepsCommand(t *testing.T) {
	controller := &fakeController{}
	m := newScreen(editableSource(testCommands()...), controller)
	load(t, m)
	listFocus(m)

	m.Update(press('x', "x"))
	m.Update(press(tea.KeyEsc, ""))

	if m.confirming || m.confirmName != "" {
		t.Fatal("esc did not cancel the confirmation")
	}
	if len(controller.removeCalls) != 0 {
		t.Fatalf("remove called after cancel: %v", controller.removeCalls)
	}
}

func TestRemoveErrorIsShown(t *testing.T) {
	controller := &fakeController{removeErr: errors.New("boom")}
	m := newScreen(editableSource(testCommands()...), controller)
	load(t, m)
	listFocus(m)

	m.Update(press('x', "x"))
	_, cmd := m.Update(press(tea.KeyEnter, ""))
	m.Update(cmd())

	if !strings.Contains(ansi.Strip(m.View()), "could not remove command: boom") {
		t.Fatalf("remove error is missing:\n%s", m.View())
	}
}

func TestToggleDisablesSelectedCommand(t *testing.T) {
	controller := &fakeController{}
	m := newScreen(editableSource(testCommands()...), controller)
	load(t, m)
	listFocus(m)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if cmd == nil {
		t.Fatal("space returned no command")
	}
	msg, ok := cmd().(toggledMsg)
	if !ok {
		t.Fatalf("message = %T, want toggledMsg", cmd())
	}
	m.Update(msg)

	if len(controller.toggleCalls) != 1 {
		t.Fatalf("toggle calls = %v, want one", controller.toggleCalls)
	}
	if got := controller.toggleCalls[0]; got.name != "code" || !got.disabled {
		t.Fatalf("toggle call = %+v, want code disabled", got)
	}
	command, _ := m.commandByName("code")
	if !command.Disabled {
		t.Fatal("command was not marked disabled")
	}
	if !strings.Contains(ansi.Strip(m.View()), `command "code" disabled.`) {
		t.Fatalf("toggle status is missing:\n%s", m.View())
	}
}

func TestToggleErrorIsShown(t *testing.T) {
	controller := &fakeController{toggleErr: errors.New("boom")}
	m := newScreen(editableSource(testCommands()...), controller)
	load(t, m)
	listFocus(m)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m.Update(cmd())

	if !strings.Contains(ansi.Strip(m.View()), "could not disable command: boom") {
		t.Fatalf("toggle error is missing:\n%s", m.View())
	}
}

func TestMoveAndScrollTracksSelection(t *testing.T) {
	items := make([]Command, 10)
	for i := range items {
		items[i] = Command{Name: fmt.Sprintf("c%02d", i), Template: "true"}
	}
	m := newScreen(editableSource(items...), &fakeController{})
	m.Resize(60, 8)
	load(t, m)
	listFocus(m)

	for range 4 {
		m.Update(press(tea.KeyDown, ""))
	}

	if m.selected != 4 {
		t.Fatalf("selected = %d, want 4", m.selected)
	}
	if m.offset != 2 {
		t.Fatalf("offset = %d, want 2", m.offset)
	}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "c00") || !strings.Contains(view, "c02") || !strings.Contains(view, "c04") {
		t.Fatalf("view does not show the scrolled window:\n%s", view)
	}
}

func TestViewFitsContentSize(t *testing.T) {
	m := newScreen(editableSource(testCommands()...), &fakeController{})
	m.Resize(48, 12)
	load(t, m)

	lines := strings.Split(m.View(), "\n")
	if len(lines) != 12 {
		t.Fatalf("rendered %d lines, want 12", len(lines))
	}
	for i, line := range lines {
		if width := lipgloss.Width(line); width > 48 {
			t.Fatalf("line %d width = %d, want <= 48", i, width)
		}
	}
}
