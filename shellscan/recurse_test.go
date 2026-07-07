package shellscan

import "testing"

func TestScanRecursesDashC(t *testing.T) {
	r := Scan(`bash -c 'approve --yes'`)
	found := false
	for _, i := range r.Invocations {
		if i.Name.Text == "approve" {
			found = true
		}
	}
	if !found {
		t.Fatalf("inner approve not found: %+v", r.Invocations)
	}
	if Scan(`sh -c 'echo "unclosed'`).ParseErr != true {
		t.Fatal("inner parse failure must set ParseErr")
	}
	// non-literal payload: out of scope, no ParseErr, no recursion
	r = Scan(`bash -c "$CMD"`)
	if r.ParseErr || len(r.Invocations) != 1 {
		t.Fatalf("variable payload: %+v", r)
	}
	// nested recursion
	r = Scan(`bash -c "sh -c 'gate-disable'"`)
	found = false
	for _, i := range r.Invocations {
		if i.Name.Text == "gate-disable" {
			found = true
		}
	}
	if !found {
		t.Fatal("nested -c not scanned")
	}
}
