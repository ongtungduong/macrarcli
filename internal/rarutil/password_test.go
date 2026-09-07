package rarutil

import (
	"errors"
	"testing"
)

// fakePrompter fakes ttyPrompter so ResolvePassword's branching can be
// exercised without a real terminal: masked-input reading itself is a
// thin, well-tested stdlib-adjacent call not worth re-testing here.
type fakePrompter struct {
	isTerminal bool
	typed      string
	readErr    error
	called     bool
}

func (f *fakePrompter) IsTerminal() bool { return f.isTerminal }

func (f *fakePrompter) ReadPassword() (string, error) {
	f.called = true
	if f.readErr != nil {
		return "", f.readErr
	}
	return f.typed, nil
}

func TestResolvePassword_ExplicitWinsNoPrompt(t *testing.T) {
	p := &fakePrompter{isTerminal: true, typed: "should-not-be-used"}
	got, err := ResolvePassword("explicit-pw", true, p)
	if err != nil {
		t.Fatalf("ResolvePassword: %v", err)
	}
	if got != "explicit-pw" {
		t.Errorf("password = %q, want %q", got, "explicit-pw")
	}
	if p.called {
		t.Error("ReadPassword was called despite an explicit password being given")
	}
}

func TestResolvePassword_NonEncryptedNeverPrompts(t *testing.T) {
	p := &fakePrompter{isTerminal: true, typed: "should-not-be-used"}
	got, err := ResolvePassword("", false, p)
	if err != nil {
		t.Fatalf("ResolvePassword: %v", err)
	}
	if got != "" {
		t.Errorf("password = %q, want empty", got)
	}
	if p.called {
		t.Error("ReadPassword was called for a non-encrypted archive")
	}
}

func TestResolvePassword_NonTTYFailsFastNoHang(t *testing.T) {
	p := &fakePrompter{isTerminal: false}
	_, err := ResolvePassword("", true, p)
	if !errors.Is(err, ErrPasswordRequired) {
		t.Fatalf("err = %v, want ErrPasswordRequired", err)
	}
	if p.called {
		t.Error("ReadPassword was attempted on a non-TTY")
	}
}

func TestResolvePassword_TTYPromptsAndReturnsTyped(t *testing.T) {
	p := &fakePrompter{isTerminal: true, typed: "typed-pw"}
	got, err := ResolvePassword("", true, p)
	if err != nil {
		t.Fatalf("ResolvePassword: %v", err)
	}
	if got != "typed-pw" {
		t.Errorf("password = %q, want %q", got, "typed-pw")
	}
	if !p.called {
		t.Error("ReadPassword was never called on an encrypted archive with a TTY")
	}
}

func TestResolvePassword_ReadErrorPropagates(t *testing.T) {
	boom := errors.New("read interrupted")
	p := &fakePrompter{isTerminal: true, readErr: boom}
	_, err := ResolvePassword("", true, p)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want it to wrap %v", err, boom)
	}
}
