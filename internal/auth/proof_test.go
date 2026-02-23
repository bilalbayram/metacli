package auth

import "testing"

func TestAppSecretProof(t *testing.T) {
	t.Parallel()

	proof, err := AppSecretProof("test-token", "test-secret")
	if err != nil {
		t.Fatalf("app secret proof: %v", err)
	}

	const expected = "4bd72343ca044f8aab1d98f07606cdb1cf47df0c089ff7b5b2df44e40d869970"
	if proof != expected {
		t.Fatalf("unexpected proof: got=%s want=%s", proof, expected)
	}
}
