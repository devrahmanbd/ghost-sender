package crypto

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidSignature = errors.New("invalid HMAC signature")
	ErrExpiredSignature = errors.New("signature has expired")
	ErrInvalidFormat    = errors.New("invalid signature format")
)

type HMACAlgorithm string

const (
	HMACSHA1   HMACAlgorithm = "sha1"
	HMACSHA256 HMACAlgorithm = "sha256"
	HMACSHA384 HMACAlgorithm = "sha384"
	HMACSHA512 HMACAlgorithm = "sha512"
)

type HMACManager struct {
	key       []byte
	algorithm HMACAlgorithm
}

func NewHMACManager(key []byte, algorithm HMACAlgorithm) *HMACManager {
	if algorithm == "" {
		algorithm = HMACSHA256
	}

	return &HMACManager{
		key:       key,
		algorithm: algorithm,
	}
}

func (hm *HMACManager) getHashFunc() func() hash.Hash {
	switch hm.algorithm {
	case HMACSHA1:
		return sha1.New
	case HMACSHA256:
		return sha256.New
	case HMACSHA384:
		return sha512.New384
	case HMACSHA512:
		return sha512.New
	default:
		return sha256.New
	}
}

func (hm *HMACManager) Sign(data []byte) []byte {
	h := hmac.New(hm.getHashFunc(), hm.key)
	h.Write(data)
	return h.Sum(nil)
}

func (hm *HMACManager) SignString(data string) string {
	signature := hm.Sign([]byte(data))
	return hex.EncodeToString(signature)
}

func (hm *HMACManager) SignBase64(data []byte) string {
	signature := hm.Sign(data)
	return base64.StdEncoding.EncodeToString(signature)
}

func (hm *HMACManager) SignURLSafe(data []byte) string {
	signature := hm.Sign(data)
	return base64.URLEncoding.EncodeToString(signature)
}

func (hm *HMACManager) Verify(data, signature []byte) bool {
	expectedSignature := hm.Sign(data)
	return hmac.Equal(signature, expectedSignature)
}

func (hm *HMACManager) VerifyString(data, signatureHex string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return hm.Verify([]byte(data), signature)
}

func (hm *HMACManager) VerifyBase64(data []byte, signatureBase64 string) bool {
	signature, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false
	}
	return hm.Verify(data, signature)
}

func (hm *HMACManager) VerifyURLSafe(data []byte, signatureBase64 string) bool {
	signature, err := base64.URLEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false
	}
	return hm.Verify(data, signature)
}

func (hm *HMACManager) SignWithTimestamp(data []byte) ([]byte, int64) {
	timestamp := time.Now().Unix()
	message := append(data, []byte(fmt.Sprintf("%d", timestamp))...)
	signature := hm.Sign(message)
	return signature, timestamp
}

func (hm *HMACManager) VerifyWithTimestamp(data, signature []byte, timestamp int64, maxAge time.Duration) error {
	if maxAge > 0 {
		age := time.Since(time.Unix(timestamp, 0))
		if age > maxAge {
			return ErrExpiredSignature
		}
	}

	message := append(data, []byte(fmt.Sprintf("%d", timestamp))...)
	if !hm.Verify(message, signature) {
		return ErrInvalidSignature
	}

	return nil
}

func Sign(data, key []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func SignString(data, key string) string {
	signature := Sign([]byte(data), []byte(key))
	return hex.EncodeToString(signature)
}

func Verify(data, signature, key []byte) bool {
	expectedSignature := Sign(data, key)
	return hmac.Equal(signature, expectedSignature)
}

func VerifyString(data, signatureHex, key string) bool {
	signature, err := hex.DecodeString(signatureHex)
	if err != nil {
		return false
	}
	return Verify([]byte(data), signature, []byte(key))
}

func SignURL(baseURL string, params map[string]string, key []byte) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	q := u.Query()
	var keys []string
	for k, v := range params {
		q.Set(k, v)
		keys = append(keys, k)
	}

	message := baseURL
	for _, k := range keys {
		message += k + q.Get(k)
	}

	hm := NewHMACManager(key, HMACSHA256)
	signature := hm.SignString(message)
	q.Set("signature", signature)

	u.RawQuery = q.Encode()
	return u.String(), nil
}

func VerifyURL(fullURL string, key []byte) (bool, error) {
	u, err := url.Parse(fullURL)
	if err != nil {
		return false, err
	}

	q := u.Query()
	providedSignature := q.Get("signature")
	if providedSignature == "" {
		return false, ErrInvalidFormat
	}

	q.Del("signature")
	u.RawQuery = q.Encode()

	message := u.String()
	hm := NewHMACManager(key, HMACSHA256)
	expectedSignature := hm.SignString(message)

	return hmac.Equal([]byte(providedSignature), []byte(expectedSignature)), nil
}

func GenerateUnsubscribeToken(email, campaignID string, key []byte, expiration time.Duration) string {
	timestamp := time.Now().Unix()
	expiresAt := time.Now().Add(expiration).Unix()
	
	data := fmt.Sprintf("%s:%s:%d:%d", email, campaignID, timestamp, expiresAt)
	hm := NewHMACManager(key, HMACSHA256)
	signature := hm.SignString(data)

	token := base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s.%s", data, signature)))
	return token
}

func ValidateUnsubscribeToken(token string, key []byte) (email, campaignID string, err error) {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", "", ErrInvalidFormat
	}

	parts := strings.SplitN(string(decoded), ".", 2)
	if len(parts) != 2 {
		return "", "", ErrInvalidFormat
	}

	data := parts[0]
	signature := parts[1]

	hm := NewHMACManager(key, HMACSHA256)
	if !hm.VerifyString(data, signature) {
		return "", "", ErrInvalidSignature
	}

	dataParts := strings.Split(data, ":")
	if len(dataParts) != 4 {
		return "", "", ErrInvalidFormat
	}

	email = dataParts[0]
	campaignID = dataParts[1]
	expiresAt, err := strconv.ParseInt(dataParts[3], 10, 64)
	if err != nil {
		return "", "", ErrInvalidFormat
	}

	if time.Now().Unix() > expiresAt {
		return "", "", ErrExpiredSignature
	}

	return email, campaignID, nil
}

func SignPayload(payload map[string]interface{}, key []byte) (string, error) {
	var parts []string
	for k, v := range payload {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}

	message := strings.Join(parts, "&")
	hm := NewHMACManager(key, HMACSHA256)
	return hm.SignString(message), nil
}

func VerifyPayload(payload map[string]interface{}, signature string, key []byte) bool {
	expectedSignature, err := SignPayload(payload, key)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func GenerateWebhookSignature(payload []byte, secret string, timestamp int64) string {
	message := fmt.Sprintf("%d.%s", timestamp, string(payload))
	hm := NewHMACManager([]byte(secret), HMACSHA256)
	return hm.SignString(message)
}

func VerifyWebhookSignature(payload []byte, signature, secret string, timestamp int64, tolerance time.Duration) error {
	expectedSignature := GenerateWebhookSignature(payload, secret, timestamp)
	
	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return ErrInvalidSignature
	}

	if tolerance > 0 {
		age := time.Since(time.Unix(timestamp, 0))
		if age > tolerance {
			return ErrExpiredSignature
		}
	}

	return nil
}

func ComputeMAC(message []byte, key []byte, algorithm HMACAlgorithm) []byte {
	var h hash.Hash

	switch algorithm {
	case HMACSHA1:
		h = hmac.New(sha1.New, key)
	case HMACSHA256:
		h = hmac.New(sha256.New, key)
	case HMACSHA384:
		h = hmac.New(sha512.New384, key)
	case HMACSHA512:
		h = hmac.New(sha512.New, key)
	default:
		h = hmac.New(sha256.New, key)
	}

	h.Write(message)
	return h.Sum(nil)
}

func ValidateMAC(message, messageMAC, key []byte, algorithm HMACAlgorithm) bool {
	expectedMAC := ComputeMAC(message, key, algorithm)
	return hmac.Equal(messageMAC, expectedMAC)
}
