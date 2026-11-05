package cloud

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"testing"
)

// Fixed vector generated independently with Python hashlib and a standalone RC4 implementation.
func TestProtocolVector(t *testing.T) {
	v, key, e := encryptRequest("/miotspec/prop/get", `{"params":[]}`, "MDEyMzQ1Njc4OWFiY2RlZg==", "AAECAwQFBgcICQoL")
	if e != nil {
		t.Fatal(e)
	}
	for k, want := range map[string]string{"data": "5bCmozmqi3E4c346qQ==", "rc4_hash__": "3daAlQCHvE5CJ3A3v81DzZe26uDwd2CPHZethQ==", "signature": "Wphsy6u40ooFj4J2gyWs3XeHul4="} {
		if v.Get(k) != want {
			t.Fatalf("%s vector mismatch", k)
		}
	}
	if base64.StdEncoding.EncodeToString(key) != "16/CeTzC9IqVVbiZ01Hy/Qd8rtVo5ybLo+ph/Vvh52k=" {
		t.Fatal("signed nonce mismatch")
	}
	if _, _, e = encryptRequest("/x", "{}", "bad", "bad"); e == nil {
		t.Fatal("accepted malformed credential material")
	}
}

func TestEncryptedGzipAndResourceLimit(t *testing.T) {
	key := []byte("0123456789abcdef")
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(`{"code":0,"result":[]}`))
	_ = w.Close()
	enc, e := crypt(key, b.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	got, e := decryptResponse(key, []byte(base64.StdEncoding.EncodeToString(enc)))
	if e != nil || string(got) != `{"code":0,"result":[]}` {
		t.Fatalf("gzip decrypt failed: %v", e)
	}
	if _, e = readBounded(bytes.NewReader(make([]byte, 33)), 32); e == nil {
		t.Fatal("body limit not enforced")
	}
}
