package screen

// Stack is the navigation stack of the UI. The bottom screen is the root and
// cannot be popped; every other screen sits on top of it.
type Stack struct {
	items []Screen
}

// NewStack returns a stack with root at the bottom.
func NewStack(root Screen) *Stack {
	return &Stack{items: []Screen{root}}
}

// Push adds s on top of the stack.
func (s *Stack) Push(screen Screen) {
	s.items = append(s.items, screen)
}

// Pop removes the top screen. It returns false when only the root is left.
func (s *Stack) Pop() (Screen, bool) {
	if len(s.items) <= 1 {
		return nil, false
	}
	popped := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return popped, true
}

// Top returns the active screen.
func (s *Stack) Top() Screen {
	if len(s.items) == 0 {
		return nil
	}
	return s.items[len(s.items)-1]
}

// ReplaceTop replaces the active screen with updated, preserving the rest of
// the stack.
func (s *Stack) ReplaceTop(updated Screen) {
	if len(s.items) == 0 {
		s.items = []Screen{updated}
		return
	}
	s.items[len(s.items)-1] = updated
}

// Len reports the number of screens on the stack.
func (s *Stack) Len() int {
	return len(s.items)
}
