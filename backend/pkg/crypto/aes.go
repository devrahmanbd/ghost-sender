package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	AES128KeySize = 16
	AES192KeySize = 24
	AES256KeySize = 32
	DefaultKeySize = AES256KeySize
	NonceSize = 12
	SaltSize = 32
	PBKDF2Iterations = 100000
)

var (
	ErrInvalidKeySize = errors.New("invalid key size")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrInvalidNonce = errors.New("invalid nonce")
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrDecryptionFailed = errors.New("decryption failed")
)

type AESCipher struct {
	key []byte
}

func NewAESCipher(key []byte) (*AESCipher, error) {
	if len(key) != AES128KeySize && len(key) != AES192KeySize && len(key) != AES256KeySize {
		return nil, ErrInvalidKeySize
	}

	return &AESCipher{
		key: key,
	}, nil
}

func NewAESCipherWithPassword(password string, salt []byte) (*AESCipher, error) {
	if len(salt) == 0 {
		salt = make([]byte, SaltSize)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, err
		}
	}

	key := DeriveKey(password, salt, DefaultKeySize)
	return NewAESCipher(key)
}

func GenerateKey(size int) ([]byte, error) {
	if size != AES128KeySize && size != AES192KeySize && size != AES256KeySize {
		return nil, ErrInvalidKeySize
	}

	key := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}

	return key, nil
}

func DeriveKey(password string, salt []byte, keySize int) []byte {
	return pbkdf2.Key([]byte(password), salt, PBKDF2Iterations, keySize, sha256.New)
}

func (c *AESCipher) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

func (c *AESCipher) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, ErrInvalidCiphertext
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

func (c *AESCipher) EncryptString(plaintext string) (string, error) {
	ciphertext, err := c.Encrypt([]byte(plaintext))
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (c *AESCipher) DecryptString(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	plaintext, err := c.Decrypt(data)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

func (c *AESCipher) EncryptWithNonce(plaintext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, ErrInvalidNonce
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	return ciphertext, nil
}

func (c *AESCipher) DecryptWithNonce(ciphertext, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(nonce) != gcm.NonceSize() {
		return nil, ErrInvalidNonce
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

func EncryptBytes(plaintext, key []byte) ([]byte, error) {
	cipher, err := NewAESCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.Encrypt(plaintext)
}

func DecryptBytes(ciphertext, key []byte) ([]byte, error) {
	cipher, err := NewAESCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.Decrypt(ciphertext)
}

func EncryptString(plaintext string, key []byte) (string, error) {
	cipher, err := NewAESCipher(key)
	if err != nil {
		return "", err
	}

	return cipher.EncryptString(plaintext)
}

func DecryptString(ciphertext string, key []byte) (string, error) {
	cipher, err := NewAESCipher(key)
	if err != nil {
		return "", err
	}

	return cipher.DecryptString(ciphertext)
}

func EncryptWithPassword(plaintext, password string, salt []byte) (string, []byte, error) {
	if len(salt) == 0 {
		salt = make([]byte, SaltSize)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return "", nil, err
		}
	}

	key := DeriveKey(password, salt, DefaultKeySize)
	cipher, err := NewAESCipher(key)
	if err != nil {
		return "", nil, err
	}

	encrypted, err := cipher.EncryptString(plaintext)
	if err != nil {
		return "", nil, err
	}

	return encrypted, salt, nil
}

func DecryptWithPassword(ciphertext, password string, salt []byte) (string, error) {
	key := DeriveKey(password, salt, DefaultKeySize)
	cipher, err := NewAESCipher(key)
	if err != nil {
		return "", err
	}

	return cipher.DecryptString(ciphertext)
}

type EncryptedData struct {
	Ciphertext []byte `json:"ciphertext"`
	Nonce      []byte `json:"nonce"`
	Salt       []byte `json:"salt,omitempty"`
}

func (c *AESCipher) EncryptData(plaintext []byte) (*EncryptedData, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &EncryptedData{
		Ciphertext: ciphertext,
		Nonce:      nonce,
	}, nil
}

func (c *AESCipher) DecryptData(data *EncryptedData) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := gcm.Open(nil, data.Nonce, data.Ciphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return nonce, nil
}

func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}

type CipherMode int

const (
	ModeCBC CipherMode = iota
	ModeCTR
	ModeGCM
)

func EncryptCBC(plaintext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	plaintext = PKCS7Pad(plaintext, aes.BlockSize)

	ciphertext := make([]byte, len(plaintext))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, plaintext)

	return ciphertext, nil
}

func DecryptCBC(ciphertext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, ErrInvalidCiphertext
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	plaintext, err = PKCS7Unpad(plaintext, aes.BlockSize)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func EncryptCTR(plaintext, key []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	ciphertext := make([]byte, len(plaintext))
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, nil, err
	}

	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(ciphertext, plaintext)

	return ciphertext, iv, nil
}

func DecryptCTR(ciphertext, key, iv []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	plaintext := make([]byte, len(ciphertext))
	stream := cipher.NewCTR(block, iv)
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}

func PKCS7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := make([]byte, padding)
	for i := range padtext {
		padtext[i] = byte(padding)
	}
	return append(data, padtext...)
}

func PKCS7Unpad(data []byte, blockSize int) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("invalid padding size")
	}

	padding := int(data[length-1])
	if padding > blockSize || padding > length {
		return nil, errors.New("invalid padding")
	}

	for i := length - padding; i < length; i++ {
		if data[i] != byte(padding) {
			return nil, errors.New("invalid padding")
		}
	}

	return data[:length-padding], nil
}

func KeyFromString(keyStr string) ([]byte, error) {
	key := []byte(keyStr)
	keyLen := len(key)

	if keyLen == AES128KeySize || keyLen == AES192KeySize || keyLen == AES256KeySize {
		return key, nil
	}

	if keyLen < AES256KeySize {
		paddedKey := make([]byte, AES256KeySize)
		copy(paddedKey, key)
		return paddedKey, nil
	}

	hash := sha256.Sum256(key)
	return hash[:AES256KeySize], nil
}

func EncryptMap(data map[string]string, key []byte) (map[string]string, error) {
	cipher, err := NewAESCipher(key)
	if err != nil {
		return nil, err
	}

	encrypted := make(map[string]string)
	for k, v := range data {
		encryptedValue, err := cipher.EncryptString(v)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt key %s: %w", k, err)
		}
		encrypted[k] = encryptedValue
	}

	return encrypted, nil
}

func DecryptMap(encrypted map[string]string, key []byte) (map[string]string, error) {
	cipher, err := NewAESCipher(key)
	if err != nil {
		return nil, err
	}

	decrypted := make(map[string]string)
	for k, v := range encrypted {
		decryptedValue, err := cipher.DecryptString(v)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt key %s: %w", k, err)
		}
		decrypted[k] = decryptedValue
	}

	return decrypted, nil
}

type KeyManager struct {
	masterKey []byte
}

func NewKeyManager(masterKey []byte) (*KeyManager, error) {
	if len(masterKey) != DefaultKeySize {
		return nil, ErrInvalidKeySize
	}

	return &KeyManager{
		masterKey: masterKey,
	}, nil
}

func (km *KeyManager) DeriveKey(identifier string) []byte {
	salt := sha256.Sum256([]byte(identifier))
	return pbkdf2.Key(km.masterKey, salt[:], PBKDF2Iterations, DefaultKeySize, sha256.New)
}

func (km *KeyManager) EncryptWithIdentifier(plaintext []byte, identifier string) ([]byte, error) {
	key := km.DeriveKey(identifier)
	cipher, err := NewAESCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.Encrypt(plaintext)
}

func (km *KeyManager) DecryptWithIdentifier(ciphertext []byte, identifier string) ([]byte, error) {
	key := km.DeriveKey(identifier)
	cipher, err := NewAESCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.Decrypt(ciphertext)
}

func (km *KeyManager) RotateKey(newMasterKey []byte) error {
	if len(newMasterKey) != DefaultKeySize {
		return ErrInvalidKeySize
	}

	km.masterKey = newMasterKey
	return nil
}
type AES = AESCipher

func NewAES(keyStr string) (*AES, error) {
    key, err := KeyFromString(keyStr)
    if err != nil {
        return nil, err
    }
    return NewAESCipher(key)
}
