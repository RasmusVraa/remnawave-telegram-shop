package webauth

import (
	"errors"
	"testing"
	"time"
)

func TestIssuePollComplete(t *testing.T) {
	s := New(time.Minute)
	token, ttl, err := s.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || ttl != time.Minute {
		t.Fatalf("token=%q ttl=%v", token, ttl)
	}
	if _, err := s.Poll(token); !errors.Is(err, ErrPending) {
		t.Fatalf("poll pending: %v", err)
	}
	if err := s.Complete(token, Session{
		AccessToken: "a", AccessExp: time.Now().Add(time.Minute),
		RefreshToken: "r", CSRFToken: "c", AccountID: 7,
	}); err != nil {
		t.Fatal(err)
	}
	sess, err := s.Poll(token)
	if err != nil {
		t.Fatal(err)
	}
	if sess.AccountID != 7 || sess.AccessToken != "a" {
		t.Fatalf("session=%+v", sess)
	}
	if _, err := s.Poll(token); !errors.Is(err, ErrNotFound) && !errors.Is(err, ErrConsumed) {
		t.Fatalf("second poll: %v", err)
	}
}

func TestTokenFromStartArg(t *testing.T) {
	tok, ok := TokenFromStartArg("webauth_deadbeef")
	if !ok || tok != "deadbeef" {
		t.Fatalf("got %q %v", tok, ok)
	}
	if _, ok := TokenFromStartArg("ref_1"); ok {
		t.Fatal("unexpected ok")
	}
	if _, ok := TokenFromStartArg("webauth_"); ok {
		t.Fatal("empty token")
	}
}

func TestExpired(t *testing.T) {
	s := New(time.Millisecond)
	token, _, err := s.Issue()
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	if err := s.Complete(token, Session{AccessToken: "a", AccountID: 1}); !errors.Is(err, ErrExpired) {
		t.Fatalf("complete: %v", err)
	}
}
