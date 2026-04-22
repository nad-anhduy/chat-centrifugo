package userkeypair

import "testing"

func TestEncryptDecryptRoundTrip(t *testing.T) {
	const master = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	plain := "-----BEGIN PRIVATE KEY-----\nABC\n-----END PRIVATE KEY-----\n"

	enc, err := EncryptPEMWithMasterKey(master, plain)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DecryptPEMWithMasterKey(master, enc)
	if err != nil {
		t.Fatal(err)
	}
	if out != plain {
		t.Fatalf("got %q want %q", out, plain)
	}
}

func TestGenerateRSA2048PEM(t *testing.T) {
	pub, priv, err := GenerateRSA2048PEM()
	if err != nil {
		t.Fatal(err)
	}
	if len(pub) < 100 || len(priv) < 100 {
		t.Fatalf("unexpected PEM lengths pub=%d priv=%d", len(pub), len(priv))
	}
}
