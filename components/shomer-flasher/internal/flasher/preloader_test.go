package flasher

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type replyChunk struct {
	data string
	err  error
}

type chunkedReply struct{ chunks []replyChunk }

func (r *chunkedReply) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	c := r.chunks[0]
	r.chunks = r.chunks[1:]
	return copy(p, c.data), c.err
}

func TestCollectPreloaderReplyJoinsPartialReadsAcrossTimeout(t *testing.T) {
	r := &chunkedReply{chunks: []replyChunk{
		{data: "READY", err: errors.New("read timeout")},
		{data: "T", err: errors.New("read timeout")},
		{data: "OO"},
	}}
	got, err := collectPreloaderReply(r, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "READYTOO" {
		t.Fatalf("reply %q", got)
	}
}

func TestCollectPreloaderReplyReturnsNoAnswer(t *testing.T) {
	_, err := collectPreloaderReply(strings.NewReader(""), time.Millisecond)
	if err == nil || err.Error() != "no answer from preloader" {
		t.Fatalf("error %v", err)
	}
}
