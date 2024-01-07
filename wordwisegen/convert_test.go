package main

import "testing"

func TestReplacementMediator(t *testing.T) {
	m := newReplacementMediator(10)
	word := "doesntmatter"
	if m.hasReplacedJustNow(word, 20) {
		t.Fatalf("no, %s hasn't been replaced before", word)
	}
	m.setLastReplacedPosition(word, 1)
	if !m.hasReplacedJustNow(word, 8) {
		t.Fatalf("%s has just been replaced at 1. next postion must be >= 11", word)
	}
	if m.hasReplacedJustNow(word, 12) {
		t.Fatalf("%s was replaced at 1 is expected to be replaced at postion >= 11 (12 > 11)", word)
	}
}
