package library

import (
	"fmt"
	"strings"
	"time"
)

type Command struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Command      string     `json:"command"`
	Description  string     `json:"description"`
	Explanation  string     `json:"explanation"`
	Tags         []string   `json:"tags"`
	Host         string     `json:"host"`
	Risk         string     `json:"risk"`
	Prerequisite string     `json:"prerequisite"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	UseCount     uint64     `json:"use_count"`
}

func (c Command) Validate() error {
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Command) == "" {
		return fmt.Errorf("id, name, and command are required")
	}
	switch c.Risk {
	case "READ", "WRITE", "DANGER", "CONNECT":
	default:
		return fmt.Errorf("risk must be READ, WRITE, DANGER, or CONNECT")
	}
	if strings.ContainsRune(c.Command, 0) {
		return fmt.Errorf("command cannot contain NUL")
	}
	return nil
}

// Seeds are harmless examples for a new library only.
func Seeds() []Command {
	now := time.Now().UTC()
	cs := []Command{
		{ID: "working-directory", Name: "Working Directory", Command: "pwd", Description: "Shows the current working directory.", Explanation: "pwd prints the shell's current directory. It does not change directories or files.", Tags: []string{"files", "directory"}, Host: "local", Risk: "READ"},
		{ID: "list-files", Name: "List Files", Command: "ls -lah", Description: "Lists files, including hidden entries.", Explanation: "ls lists directory entries. -l shows details, -a includes hidden entries, and -h formats sizes for reading. This reads directory metadata.", Tags: []string{"files", "directory"}, Host: "local", Risk: "READ"},
		{ID: "utc-time", Name: "UTC Time", Command: "date -u", Description: "Shows the current date and time in UTC.", Explanation: "date prints the current time. -u selects UTC. No date-setting argument is supplied.", Tags: []string{"time"}, Host: "local", Risk: "READ"},
		{ID: "git-status", Name: "Git Status", Command: "git status --short", Description: "Shows a compact summary of Git working tree changes.", Explanation: "git status reports tracked changes and untracked files. --short requests compact output. It does not stage or commit changes.", Tags: []string{"git", "files"}, Host: "local", Risk: "READ", Prerequisite: "Git installed; run inside a Git repository."},
		{ID: "hello", Name: "Print Hello", Command: "printf 'Hello from cmdlib\\n'", Description: "Prints a greeting to the terminal.", Explanation: "printf writes the given format to standard output. The escaped newline ends the greeting. It creates no files.", Tags: []string{"terminal", "example"}, Host: "local", Risk: "READ"},
	}
	for i := range cs {
		cs[i].CreatedAt = now
		cs[i].UpdatedAt = now
	}
	return cs
}
