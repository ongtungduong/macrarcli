package rarutil

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

// ErrPasswordRequired reports that an archive's header is encrypted, no
// explicit password was given, and stdin is not a terminal to prompt on.
// Callers (the CLI layer) map this to a distinct exit code rather than
// hanging or silently guessing an empty password.
var ErrPasswordRequired = errors.New("password required: archive header is encrypted and stdin is not a terminal")

// ttyPrompter isolates the terminal calls ResolvePassword needs behind an
// interface, so tests can fake "is a TTY" / "user typed X" without a real
// terminal (masked input can't be exercised in an automated test without a
// pty, so only the branching logic below is unit-tested against a fake).
type ttyPrompter interface {
	IsTerminal() bool
	ReadPassword() (string, error)
}

// stdinPrompter is the real ttyPrompter, backed by os.Stdin.
type stdinPrompter struct{}

func (stdinPrompter) IsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

func (stdinPrompter) ReadPassword() (string, error) {
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(b), nil
}

// ResolvePassword determines the password to use for a batch. It must be
// called ONCE per CLI invocation, before RunBatch dispatches any job: an
// interactive prompt run inside a per-job goroutine would race on shared
// stdin/tty state, since Options (including Password) applies uniformly to
// every job in a batch.
//
// explicit always wins with no prompt attempted. If headerEncrypted is
// false, no prompt is attempted even when explicit is empty (a plain archive
// never needs a password). Otherwise it prompts on a TTY and masks input;
// on a non-TTY it fails fast with ErrPasswordRequired instead of hanging.
func ResolvePassword(explicit string, headerEncrypted bool, p ttyPrompter) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if !headerEncrypted {
		return "", nil
	}
	if !p.IsTerminal() {
		return "", ErrPasswordRequired
	}
	return p.ReadPassword()
}

// ResolvePasswordStdin is ResolvePassword against the real terminal
// (os.Stdin), the entry point the CLI layer calls.
func ResolvePasswordStdin(explicit string, headerEncrypted bool) (string, error) {
	return ResolvePassword(explicit, headerEncrypted, stdinPrompter{})
}
