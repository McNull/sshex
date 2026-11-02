package screen

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type stubScreen struct {
	name string
}

func (s stubScreen) Init() tea.Cmd                    { return nil }
func (s stubScreen) Update(tea.Msg) (Screen, tea.Cmd) { return s, nil }
func (s stubScreen) View() string                     { return s.name }
func (s stubScreen) Title() string                    { return s.name }
func (s stubScreen) Keys() []key.Binding              { return nil }
func (s stubScreen) Resize(int, int)                  {}

func TestStackPushPop(t *testing.T) {
	root := stubScreen{name: "root"}
	stack := NewStack(root)
	if stack.Len() != 1 || stack.Top() != root {
		t.Fatalf("new stack = %v, want root", stack.Top())
	}

	child := stubScreen{name: "child"}
	stack.Push(child)
	if stack.Len() != 2 || stack.Top() != child {
		t.Fatalf("after push top = %v, want child", stack.Top())
	}

	popped, ok := stack.Pop()
	if !ok || popped != child {
		t.Fatalf("pop = (%v, %v), want child, true", popped, ok)
	}
	if stack.Top() != root {
		t.Fatalf("after pop top = %v, want root", stack.Top())
	}

	if _, ok := stack.Pop(); ok {
		t.Fatal("popping the root should fail")
	}
	if stack.Len() != 1 {
		t.Fatalf("stack length = %d, want 1", stack.Len())
	}
}

func TestStackReplaceTop(t *testing.T) {
	stack := NewStack(stubScreen{name: "root"})
	updated := stubScreen{name: "updated"}
	stack.ReplaceTop(updated)
	if stack.Top() != updated {
		t.Fatalf("top = %v, want updated", stack.Top())
	}
}
