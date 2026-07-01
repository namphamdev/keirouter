package qoder

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// decodeBody reverses EncodeBody: undo the custom substitution, undo the
// [tail][mid][head] rearrangement, then base64-decode. It mirrors what the
// Qoder server does when it receives a body with &Encode=1.
func decodeBody(t *testing.T, encoded []byte) []byte {
	t.Helper()
	if len(encoded) == 0 {
		return nil
	}

	// Build the inverse substitution table (custom -> standard).
	var c2s [128]byte
	for i := range c2s {
		c2s[i] = byte(i)
	}
	for i := 0; i < 64; i++ {
		c2s[customAlphabet[i]] = stdAlphabet[i]
	}
	c2s['$'] = '='

	// Step 3 inverse: substitute back.
	sub := make([]byte, len(encoded))
	for i, c := range encoded {
		if c < 128 {
			sub[i] = c2s[c]
		} else {
			sub[i] = c
		}
	}

	// Step 2 inverse: rearranged = tail(a) + mid(n-2a) + head(a).
	// Recover encoded = head + mid + tail.
	n := len(sub)
	a := n / 3
	var b64 []byte
	b64 = append(b64, sub[n-a:]...)  // head
	b64 = append(b64, sub[a:n-a]...) // mid
	b64 = append(b64, sub[:a]...)    // tail

	// Step 1 inverse: base64 decode.
	out, err := base64.StdEncoding.DecodeString(string(b64))
	if err != nil {
		t.Fatalf("decodeBody: base64 decode failed: %v", err)
	}
	return out
}

func TestEncodeBodyRoundTrip(t *testing.T) {
	cases := [][]byte{
		[]byte("a"),
		[]byte("ab"),
		[]byte("abc"),
		[]byte(`{"model":"qoder","messages":[{"role":"user","content":"hello world"}]}`),
		[]byte("The quick brown fox jumps over the lazy dog 1234567890"),
		bytes.Repeat([]byte("x"), 1000),
		[]byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x80, 0x7f},
	}

	for i, plain := range cases {
		encoded := EncodeBody(plain)
		got := decodeBody(t, encoded)
		if !bytes.Equal(got, plain) {
			t.Fatalf("case %d: round-trip mismatch\n got: %q\nwant: %q", i, got, plain)
		}
	}
}

func TestEncodeBodyEmpty(t *testing.T) {
	if got := EncodeBody(nil); got != nil {
		t.Fatalf("EncodeBody(nil) = %v, want nil", got)
	}
	if got := EncodeBody([]byte{}); got != nil {
		t.Fatalf("EncodeBody(empty) = %v, want nil", got)
	}
}

func TestEncodeBodyIsLatin1Safe(t *testing.T) {
	// The output must contain only bytes < 128 so it survives latin1 transport.
	plain := []byte("some payload with bytes \xff\xfe and text")
	encoded := EncodeBody(plain)
	for i, b := range encoded {
		if b >= 128 {
			t.Fatalf("output byte %d = %d is not < 128", i, b)
		}
	}
}

func TestEncodeBodyDeterministic(t *testing.T) {
	plain := []byte("determinism check")
	a := EncodeBody(plain)
	b := EncodeBody(plain)
	if !bytes.Equal(a, b) {
		t.Fatal("EncodeBody is not deterministic for the same input")
	}
}

func TestEncodeBodyChangesInput(t *testing.T) {
	// Encoded output should differ from the raw base64 (obfuscation applied).
	plain := []byte("this should be obfuscated")
	encoded := EncodeBody(plain)
	rawB64 := base64.StdEncoding.EncodeToString(plain)
	if string(encoded) == rawB64 {
		t.Fatal("EncodeBody output equals plain base64, expected obfuscation")
	}
}
