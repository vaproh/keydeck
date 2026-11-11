package auth

import "testing"

func TestPasswordHashRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyArgon2(hash, "correct horse battery staple") {
		t.Fatal("valid password rejected")
	}
	if verifyArgon2(hash, "wrong") {
		t.Fatal("invalid password accepted")
	}
}
