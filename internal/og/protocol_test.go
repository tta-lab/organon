package og

import (
	"encoding/json"
	"testing"
)

func TestRequestJSONDistinguishesAbsentAndEmptyMutationFields(t *testing.T) {
	empty := ""
	data, err := json.Marshal(Request{Index: 7, Body: &empty})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["title"]; ok {
		t.Fatalf("absent title was encoded: %s", data)
	}
	if body, ok := got["body"]; !ok || body != "" {
		t.Fatalf("empty body was not encoded explicitly: %s", data)
	}
}

func TestResponseDefaultsToOKWhenHandlerSucceeds(t *testing.T) {
	resp := success(Response{Message: "done"})
	if !resp.OK {
		t.Fatal("success response did not set OK")
	}
	if resp.Message != "done" {
		t.Fatalf("message = %q", resp.Message)
	}
}

func TestCloneRequestJSONHasNoWorktreeOrCredentialFields(t *testing.T) {
	data, err := json.Marshal(Request{URL: "https://codeberg.org/tta-lab/example.git", Alias: "example"})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["url"] != "https://codeberg.org/tta-lab/example.git" || got["alias"] != "example" {
		t.Fatalf("clone request = %s", data)
	}
	for _, forbidden := range []string{"work_dir", "path", "cwd", "root", "token", "token_env"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("clone request includes %q: %s", forbidden, data)
		}
	}
}
