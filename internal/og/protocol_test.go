package og

import "testing"

func TestResponseDefaultsToOKWhenHandlerSucceeds(t *testing.T) {
	resp := success(Response{Message: "done"})
	if !resp.OK {
		t.Fatal("success response did not set OK")
	}
	if resp.Message != "done" {
		t.Fatalf("message = %q", resp.Message)
	}
}
