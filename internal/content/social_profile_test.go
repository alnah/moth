package content

import "testing"

func TestKindSocialProfileIsStable(t *testing.T) {
	if got := KindSocialProfile; got != Kind("social_profile") {
		t.Fatalf("KindSocialProfile = %q, want social_profile", got)
	}
}
