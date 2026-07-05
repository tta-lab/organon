package main

import "testing"

func TestIsCI(t *testing.T) {
	if isCI() {
		t.Fatal("isCI = true without CI env")
	}
	for _, env := range []string{"CI", "GITHUB_ACTIONS", "WOODPECKER"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "1")
			if !isCI() {
				t.Fatalf("isCI = false with %s set", env)
			}
		})
	}
}
