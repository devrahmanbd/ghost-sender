package crypto

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"

	"golang.org/x/crypto/bcrypt"
)

const (
	DefaultBcryptCost = 12
	MinBcryptCost     = bcrypt.MinCost
	MaxBcryptCost     = bcrypt.MaxCost
)

var (
	ErrInvalidHash     = errors.New("invalid hash format")
	ErrHashMismatch    = errors.New("hash mismatch")
	ErrInvalidCost     = errors.New("invalid bcrypt cost")
	ErrPasswordTooLong = errors.New("password exceeds maximum length")
)

type HashAlgorithm string

const (
	AlgorithmMD5    HashAlgorithm = "md5"
	AlgorithmSHA1   HashAlgorithm = "sha1"
	AlgorithmSHA224 HashAlgorithm = "sha224"
	AlgorithmSHA256 HashAlgorithm = "sha256"
	AlgorithmSHA384 HashAlgorithm = "sha384"
	AlgorithmSHA512 HashAlgorithm = "sha512"
)

func HashPassword(password string) (string, error) {
	return HashPasswordWithCost(password, DefaultBcryptCost)
}

func HashPasswordWithCost(password string, cost int) (string, error) {
	if cost < MinBcryptCost || cost > MaxBcryptCost {
		return "", ErrInvalidCost
	}

	if len(password) > 72 {
		return "", ErrPasswordTooLong
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}

	return string(hash), nil
}

func VerifyPassword(password, hash string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func CheckPassword(password, hash string) bool {
	err := VerifyPassword(password, hash)
	return err == nil
}

func GetPasswordCost(hash string) (int, error) {
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil {
		return 0, err
	}
	return cost, nil
}

func SHA256Hash(data []byte) []byte {
	hash := sha256.Sum256(data)
	return hash[:]
}

func SHA256String(data string) string {
	hash := SHA256Hash([]byte(data))
	return hex.EncodeToString(hash)
}

func SHA256Hex(data []byte) string {
	hash := SHA256Hash(data)
	return hex.EncodeToString(hash)
}

func SHA512Hash(data []byte) []byte {
	hash := sha512.Sum512(data)
	return hash[:]
}

func SHA512String(data string) string {
	hash := SHA512Hash([]byte(data))
	return hex.EncodeToString(hash)
}

func SHA512Hex(data []byte) string {
	hash := SHA512Hash(data)
	return hex.EncodeToString(hash)
}

func SHA384Hash(data []byte) []byte {
	hash := sha512.Sum384(data)
	return hash[:]
}

func SHA384String(data string) string {
	hash := SHA384Hash([]byte(data))
	return hex.EncodeToString(hash)
}

func SHA1Hash(data []byte) []byte {
	hash := sha1.Sum(data)
	return hash[:]
}

func SHA1String(data string) string {
	hash := SHA1Hash([]byte(data))
	return hex.EncodeToString(hash)
}

func MD5Hash(data []byte) []byte {
	hash := md5.Sum(data)
	return hash[:]
}

func MD5String(data string) string {
	hash := MD5Hash([]byte(data))
	return hex.EncodeToString(hash)
}

func MD5Hex(data []byte) string {
	hash := MD5Hash(data)
	return hex.EncodeToString(hash)
}

func Hash(data []byte, algorithm HashAlgorithm) ([]byte, error) {
	switch algorithm {
	case AlgorithmMD5:
		return MD5Hash(data), nil
	case AlgorithmSHA1:
		return SHA1Hash(data), nil
	case AlgorithmSHA224:
		hash := sha256.Sum224(data)
		return hash[:], nil
	case AlgorithmSHA256:
		return SHA256Hash(data), nil
	case AlgorithmSHA384:
		return SHA384Hash(data), nil
	case AlgorithmSHA512:
		return SHA512Hash(data), nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}
}

func HashString(data string, algorithm HashAlgorithm) (string, error) {
	hash, err := Hash([]byte(data), algorithm)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash), nil
}

func HashFile(filepath string) (string, error) {
	return HashFileWithAlgorithm(filepath, AlgorithmSHA256)
}

func HashFileWithAlgorithm(filepath string, algorithm HashAlgorithm) (string, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var hasher hash.Hash

	switch algorithm {
	case AlgorithmMD5:
		hasher = md5.New()
	case AlgorithmSHA1:
		hasher = sha1.New()
	case AlgorithmSHA224:
		hasher = sha256.New224()
	case AlgorithmSHA256:
		hasher = sha256.New()
	case AlgorithmSHA384:
		hasher = sha512.New384()
	case AlgorithmSHA512:
		hasher = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}

	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func HashReader(reader io.Reader, algorithm HashAlgorithm) (string, error) {
	var hasher hash.Hash

	switch algorithm {
	case AlgorithmMD5:
		hasher = md5.New()
	case AlgorithmSHA1:
		hasher = sha1.New()
	case AlgorithmSHA224:
		hasher = sha256.New224()
	case AlgorithmSHA256:
		hasher = sha256.New()
	case AlgorithmSHA384:
		hasher = sha512.New384()
	case AlgorithmSHA512:
		hasher = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}

	if _, err := io.Copy(hasher, reader); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func CompareHash(hash1, hash2 []byte) bool {
	return subtle.ConstantTimeCompare(hash1, hash2) == 1
}

func CompareHashString(hash1, hash2 string) bool {
	return subtle.ConstantTimeCompare([]byte(hash1), []byte(hash2)) == 1
}

func ValidateHash(data []byte, hashHex string, algorithm HashAlgorithm) (bool, error) {
	expectedHash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false, ErrInvalidHash
	}

	actualHash, err := Hash(data, algorithm)
	if err != nil {
		return false, err
	}

	return CompareHash(actualHash, expectedHash), nil
}

func ValidateHashString(data, hashHex string, algorithm HashAlgorithm) (bool, error) {
	return ValidateHash([]byte(data), hashHex, algorithm)
}

func ValidateFileHash(filepath, expectedHash string, algorithm HashAlgorithm) (bool, error) {
	actualHash, err := HashFileWithAlgorithm(filepath, algorithm)
	if err != nil {
		return false, err
	}

	return CompareHashString(actualHash, expectedHash), nil
}

type MultiHasher struct {
	hashers map[HashAlgorithm]hash.Hash
}

func NewMultiHasher(algorithms ...HashAlgorithm) *MultiHasher {
	mh := &MultiHasher{
		hashers: make(map[HashAlgorithm]hash.Hash),
	}

	for _, alg := range algorithms {
		switch alg {
		case AlgorithmMD5:
			mh.hashers[alg] = md5.New()
		case AlgorithmSHA1:
			mh.hashers[alg] = sha1.New()
		case AlgorithmSHA224:
			mh.hashers[alg] = sha256.New224()
		case AlgorithmSHA256:
			mh.hashers[alg] = sha256.New()
		case AlgorithmSHA384:
			mh.hashers[alg] = sha512.New384()
		case AlgorithmSHA512:
			mh.hashers[alg] = sha512.New()
		}
	}

	return mh
}

func (mh *MultiHasher) Write(data []byte) (int, error) {
	for _, hasher := range mh.hashers {
		if _, err := hasher.Write(data); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

func (mh *MultiHasher) Sum(algorithm HashAlgorithm) ([]byte, error) {
	hasher, ok := mh.hashers[algorithm]
	if !ok {
		return nil, fmt.Errorf("algorithm %s not enabled", algorithm)
	}
	return hasher.Sum(nil), nil
}

func (mh *MultiHasher) SumHex(algorithm HashAlgorithm) (string, error) {
	sum, err := mh.Sum(algorithm)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum), nil
}

func (mh *MultiHasher) SumAll() map[HashAlgorithm][]byte {
	results := make(map[HashAlgorithm][]byte)
	for alg, hasher := range mh.hashers {
		results[alg] = hasher.Sum(nil)
	}
	return results
}

func (mh *MultiHasher) SumAllHex() map[HashAlgorithm]string {
	results := make(map[HashAlgorithm]string)
	for alg, hasher := range mh.hashers {
		results[alg] = hex.EncodeToString(hasher.Sum(nil))
	}
	return results
}

func (mh *MultiHasher) Reset() {
	for _, hasher := range mh.hashers {
		hasher.Reset()
	}
}

func HashMultiple(data []byte, algorithms ...HashAlgorithm) (map[HashAlgorithm]string, error) {
	mh := NewMultiHasher(algorithms...)
	if _, err := mh.Write(data); err != nil {
		return nil, err
	}
	return mh.SumAllHex(), nil
}

func ComputeChecksum(data []byte) string {
	return SHA256Hex(data)
}

func ComputeChecksumString(data string) string {
	return SHA256String(data)
}

func GenerateFingerprint(parts ...string) string {
	combined := ""
	for _, part := range parts {
		combined += part
	}
	return SHA256String(combined)
}

func GenerateContentHash(data []byte) string {
	return SHA256Hex(data)
}

func GenerateHashToken(data ...string) string {
	combined := ""
	for _, d := range data {
		combined += d
	}
	return SHA256String(combined)
}

func QuickHash(data []byte) uint64 {
	hash := sha256.Sum256(data)
	result := uint64(0)
	for i := 0; i < 8; i++ {
		result = (result << 8) | uint64(hash[i])
	}
	return result
}

func SecureCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func SecureCompareBytes(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

type Hasher struct {
	algorithm HashAlgorithm
	hasher    hash.Hash
}

func NewHasher(algorithm HashAlgorithm) (*Hasher, error) {
	var hasher hash.Hash

	switch algorithm {
	case AlgorithmMD5:
		hasher = md5.New()
	case AlgorithmSHA1:
		hasher = sha1.New()
	case AlgorithmSHA224:
		hasher = sha256.New224()
	case AlgorithmSHA256:
		hasher = sha256.New()
	case AlgorithmSHA384:
		hasher = sha512.New384()
	case AlgorithmSHA512:
		hasher = sha512.New()
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", algorithm)
	}

	return &Hasher{
		algorithm: algorithm,
		hasher:    hasher,
	}, nil
}

func (h *Hasher) Write(data []byte) (int, error) {
	return h.hasher.Write(data)
}

func (h *Hasher) Sum() []byte {
	return h.hasher.Sum(nil)
}

func (h *Hasher) SumHex() string {
	return hex.EncodeToString(h.Sum())
}

func (h *Hasher) Reset() {
	h.hasher.Reset()
}

func (h *Hasher) Size() int {
	return h.hasher.Size()
}

func (h *Hasher) BlockSize() int {
	return h.hasher.BlockSize()
}
