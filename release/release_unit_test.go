package release

import "testing"

func TestHighestPicksSemverMaxAndPeeledCommit(t *testing.T) {
	in := "aaa\trefs/tags/v1.9.0\n" +
		"bbb\trefs/tags/v1.10.0\n" +
		"ccc\trefs/tags/v1.10.0^{}\n" +
		"ddd\trefs/tags/v2.0.0-rc1\n" +
		"eee\trefs/tags/latest\n"
	got, err := highest(in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tag.Name != "v1.10.0" || got.Commit != "ccc" {
		t.Errorf("highest = %+v, want v1.10.0 at ccc", got)
	}
}

func TestHighestWithNoReleaseTag(t *testing.T) {
	if _, err := highest("aaa\trefs/tags/latest\n"); err != ErrNoRelease {
		t.Errorf("err = %v, want ErrNoRelease", err)
	}
}
