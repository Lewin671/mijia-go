package cloud

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/rc4"  // Required by Xiaomi's existing wire protocol, not a choice for new cryptography.
	"crypto/sha1" // Required by Xiaomi's request signature.
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net/url"
	"strings"
	"time"
)

func nonce() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b[:8]); err != nil {
		return "", err
	}
	binary.BigEndian.PutUint32(b[8:], uint32(time.Now().Unix()/60))
	return base64.StdEncoding.EncodeToString(b), nil
}

func signedNonce(security, nonce string) ([]byte, error) {
	s, err := base64.StdEncoding.DecodeString(security)
	if err != nil || len(s) == 0 {
		return nil, errors.New("invalid account security material; sign in again")
	}
	n, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return nil, errors.New("invalid request nonce")
	}
	h := sha256.Sum256(append(s, n...))
	return h[:], nil
}

func crypt(key, input []byte) ([]byte, error) {
	c, err := rc4.NewCipher(key)
	if err != nil {
		return nil, err
	}
	skip := make([]byte, 1024)
	c.XORKeyStream(skip, skip)
	out := make([]byte, len(input))
	c.XORKeyStream(out, input)
	return out, nil
}

func signature(path string, pairs []string, key []byte) string {
	parts := append([]string{"POST", path}, pairs...)
	parts = append(parts, base64.StdEncoding.EncodeToString(key))
	h := sha1.Sum([]byte(strings.Join(parts, "&")))
	return base64.StdEncoding.EncodeToString(h[:])
}

func encryptRequest(path, data, security, n string) (url.Values, []byte, error) {
	key, err := signedNonce(security, n)
	if err != nil {
		return nil, nil, err
	}
	hash := signature(path, []string{"data=" + data}, key)
	encData, err := crypt(key, []byte(data))
	if err != nil {
		return nil, nil, err
	}
	encHash, err := crypt(key, []byte(hash))
	if err != nil {
		return nil, nil, err
	}
	d, h := base64.StdEncoding.EncodeToString(encData), base64.StdEncoding.EncodeToString(encHash)
	return url.Values{"data": {d}, "rc4_hash__": {h}, "signature": {signature(path, []string{"data=" + d, "rc4_hash__=" + h}, key)}, "ssecurity": {security}, "_nonce": {n}}, key, nil
}

func decryptResponse(key []byte, body []byte) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		return nil, errors.New("cloud returned an invalid encrypted response")
	}
	b, err = crypt(key, b)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(b, []byte{0x1f, 0x8b}) {
		r, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, errors.New("invalid compressed cloud response")
		}
		defer r.Close()
		return readBounded(r, maxBody)
	}
	return b, nil
}

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("response exceeds size limit")
	}
	return b, nil
}
