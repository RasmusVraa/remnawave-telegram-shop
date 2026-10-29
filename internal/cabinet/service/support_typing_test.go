package service

import (
	"testing"
	"time"
)

func TestSupportTypingExpiresAndClears(t *testing.T) {
	s := NewSupport(nil, nil, nil, nil, nil, nil, nil, nil)
	now := time.Now()

	if s.isTyping(1, now) {
		t.Fatal("typing before any event")
	}
	s.setTyping(1, now)
	if !s.isTyping(1, now.Add(supportTypingTTL-time.Second)) {
		t.Fatal("typing should hold within TTL")
	}
	if s.isTyping(1, now.Add(supportTypingTTL)) {
		t.Fatal("typing should expire after TTL")
	}

	s.setTyping(2, now)
	s.clearTyping(2)
	if s.isTyping(2, now) {
		t.Fatal("typing should stop when the reply arrives")
	}
}

func TestSupportTypingPrunesExpired(t *testing.T) {
	s := NewSupport(nil, nil, nil, nil, nil, nil, nil, nil)
	now := time.Now()
	s.setTyping(1, now)
	s.setTyping(2, now.Add(supportTypingTTL+time.Second))
	if _, ok := s.typingUntil[1]; ok {
		t.Fatal("expired entry should be pruned on the next event")
	}
}
