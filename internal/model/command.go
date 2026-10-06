package model

// Command is a predefined command executed on the origin machine.
type Command struct {
	Name     string
	Command  string
	Alias    string
	Disabled bool
}
