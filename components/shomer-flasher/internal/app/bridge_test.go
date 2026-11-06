package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Shomer-TCL/shomer-flasher/internal/flasher"
)

// The question bridge used by the window and the TUI: whatever the person
// typed reaches the flasher (the 1.4.1 window bug lost it on the way).
func TestQuestionBridgeReturnsTypedText(t *testing.T) {
	for _, typed := range []string{"UPCI", " upci ", "KEKA", "ופבן"} {
		out := make(chan Msg, 10)
		u := &chanUI{log: flasher.NewUI(strings.NewReader(""), discard{}, discard{}), out: out, quit: make(chan struct{})}
		m := NewModel(DemoManifest(), "test", flasher.Plan{}, "")
		answered := make(chan struct{})
		go func() {
			defer close(answered)
			msg := <-out
			m.Apply(msg) // what the window/TUI does with a message
			if m.Question == nil || m.Question.Kind != QText {
				panic("no text question on screen")
			}
			m.Answer(Answer{Text: typed}) // what the OK button / Enter does
		}()
		got, err := u.Ask("    Type the 4 letters you see (KEKA/UPCI), or Q to quit:")
		if err != nil || got != strings.TrimSpace(typed) {
			t.Fatalf("typed %q, flasher got %q (%v)", typed, got, err)
		}
		<-answered
		if m.Question != nil {
			t.Fatal("question still open after the answer")
		}
	}
}

func TestQuestionBridgeCancelIsStopNotEmptyText(t *testing.T) {
	out := make(chan Msg, 10)
	u := &chanUI{log: flasher.NewUI(strings.NewReader(""), discard{}, discard{}), out: out, quit: make(chan struct{})}
	m := NewModel(DemoManifest(), "test", flasher.Plan{}, "")
	answered := make(chan struct{})
	go func() { defer close(answered); m.Apply(<-out); m.Answer(Answer{Cancel: true}) }()
	_, err := u.Ask("version?")
	<-answered
	if !errors.Is(err, flasher.ErrInputClosed) || !errors.Is(err, flasher.ErrCanceled) {
		t.Fatalf("cancel must stop the flow, got %v", err)
	}
	m.Apply(Msg{Kind: MsgDetected, Err: err})
	if m.Screen != ScreenDone || !m.Result.Refused || !strings.Contains(strings.Join(m.DoneLines(), "\n"), "STOPPED: you pressed Cancel") {
		t.Fatalf("done screen: %v", m.DoneLines())
	}
}

// A second answer to the same question (double click on OK) is ignored and
// does not block.
func TestQuestionAnsweredTwiceDoesNotBlock(t *testing.T) {
	out := make(chan Msg, 10)
	u := &chanUI{log: flasher.NewUI(strings.NewReader(""), discard{}, discard{}), out: out, quit: make(chan struct{})}
	done := make(chan string)
	go func() { s, _ := u.Ask("v?"); done <- s }()
	q := (<-out).Question
	q.reply(Answer{Text: "UPCI"})
	q.reply(Answer{Text: "KEKA"})
	select {
	case s := <-done:
		if s != "UPCI" {
			t.Fatal(s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("blocked")
	}
}

// The backup folder typed in the window may contain spaces; the model keeps
// the folder trimmed only at the ends.
func TestBackupDirWithSpaces(t *testing.T) {
	m := NewModel(DemoManifest(), "test", flasher.Plan{Backup: true}, "")
	m.SetBackupDir(`  C:\Users\Sam\My Backups  `)
	if m.Plan.BackupDir != `C:\Users\Sam\My Backups` {
		t.Fatalf("%q", m.Plan.BackupDir)
	}
}
