package sec

import (
	"crypto"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io/ioutil"
	random "math/rand"
	"os"
	"time"
)

// Generates a SHA512 hash and returns the hash.
func SHA512(data []byte) ([]byte, error) {
	hash := sha512.New()
	_, err := hash.Write(data)
	if err != nil {
		return nil, err
	}
	hashSum := hash.Sum(nil)
	return hashSum, nil
}

type RSA struct {
	PublicKey  *rsa.PublicKey
	PrivateKey *rsa.PrivateKey
}

func (context *RSA) LoadPublicKeyFromPath(path string) error {
	keyBuff, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	keyBlock, _ := pem.Decode(keyBuff)
	if keyBlock.Type != "PUBLIC KEY" {
		return errors.New("path provided to public key does not contain public key")
	}
	parsedKey, err := x509.ParsePKIXPublicKey(keyBlock.Bytes)
	if err != nil {
		return err
	}
	var publicKey *rsa.PublicKey
	publicKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("unable to parse public key")
	}
	context.PublicKey = publicKey
	return nil
}

func (context *RSA) LoadPrivateKeyFromPath(path, password string) error {
	keyBuff, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	keyBlock, _ := pem.Decode(keyBuff)
	if keyBlock.Type != "PRIVATE KEY" {
		return errors.New("path provided to private key does not contain private key")
	}
	var privateKeyBytes []byte
	if password == "" {
		privateKeyBytes = keyBuff
	} else {
		privateKeyBytes, err = x509.DecryptPEMBlock(keyBlock, []byte(password))
		if err != nil {
			return err
		}
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBytes)
	if err != nil {
		return err
	}
	context.PrivateKey = privateKey
	return nil
}

func (context *RSA) LoadPublicKeyFromBase64(key string) error {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		return err
	}
	keyBuff := []byte(decoded)
	keyBlock, _ := pem.Decode(keyBuff)
	if keyBlock.Type != "RSA PUBLIC KEY" {
		return errors.New("string does not contain public key")
	}
	parsedKey, err := x509.ParsePKIXPublicKey(keyBlock.Bytes)
	if err != nil {
		return err
	}
	var publicKey *rsa.PublicKey
	publicKey, ok := parsedKey.(*rsa.PublicKey)
	if !ok {
		return errors.New("unable to parse public key")
	}
	context.PublicKey = publicKey
	return nil
}

func (context *RSA) LoadPrivateKeyFromBase64(key, password string) error {
	keyBuff := []byte(key)
	keyBlock, _ := pem.Decode(keyBuff)
	var err error
	if keyBlock.Type != "RSA PRIVATE KEY" {
		return errors.New("string does not contain private key")
	}
	var privateKeyBytes []byte
	if password == "" {
		privateKeyBytes = keyBuff
	} else {
		privateKeyBytes, err = x509.DecryptPEMBlock(keyBlock, []byte(password))
		if err != nil {
			return err
		}
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyBytes)
	if err != nil {
		return errors.New("unable to parse private key")
	}
	context.PrivateKey = privateKey
	return nil
}

// Generates a 2048-bit keypair.
func (context *RSA) Generate() error {
	privatekey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	publickey := privatekey.PublicKey
	context.PrivateKey = privatekey
	context.PublicKey = &publickey
	return nil
}

// Encrypts a string and returns the cipher in base64.
func (context RSA) Encrypt(data []byte) ([]byte, error) {
	cipher, err := rsa.EncryptOAEP(sha512.New(), rand.Reader, context.PublicKey, data, nil)
	if err != nil {
		return nil, err
	}
	cipherBase64 := base64.StdEncoding.EncodeToString(cipher)
	return []byte(cipherBase64), nil
}

// Decrypts a string and returns the plaintext.
func (context RSA) Decrypt(cipher []byte) ([]byte, error) {
	decoded, err := base64.StdEncoding.DecodeString(string(cipher))
	if err != nil {
		return nil, err
	}
	plainText, err := context.PrivateKey.Decrypt(nil, decoded, &rsa.OAEPOptions{Hash: crypto.SHA512})
	if err != nil {
		return nil, err
	}
	return plainText, nil
}

// Signs the data and returns the signature.
func (context RSA) Sign(data []byte) ([]byte, error) {
	hash, err := SHA512(data)
	if err != nil {
		return nil, err
	}
	signature, err := rsa.SignPSS(rand.Reader, context.PrivateKey, crypto.SHA512, hash, nil)
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(signature)
	return []byte(encoded), nil
}

// Verifies the signature with the data.
func (context RSA) Verify(data, signature []byte) error {
	hash, err := SHA512(data)
	if err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(string(signature))
	if err != nil {
		return err
	}
	err = rsa.VerifyPSS(context.PublicKey, crypto.SHA512, hash, decoded, nil)
	if err != nil {
		return err
	}
	return nil
}

// Returns the encoded public key.
func (context RSA) GetPublicKey() ([]byte, error) {
	encodedKey, err := x509.MarshalPKIXPublicKey(context.PublicKey)
	if err != nil {
		return nil, err
	}

	publicKeyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: encodedKey,
	})
	encoded := base64.StdEncoding.EncodeToString(publicKeyBytes)
	return []byte(encoded), nil
}

// Retunrs the encoded private key.
func (context RSA) GetPrivateKey() []byte {
	encodedKey := x509.MarshalPKCS1PrivateKey(context.PrivateKey)
	privateKeyBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: encodedKey,
	})
	encoded := base64.StdEncoding.EncodeToString(privateKeyBytes)
	return []byte(encoded)
}

type AES struct {
	AESkey []byte
}

// Generates a random aes key.
func (context *AES) Generate() error {
	key := make([]byte, 64)
	_, err := rand.Read(key)
	if err != nil {
		return err
	}
	context.AESkey = key
	return nil
}

// Loads the aes key.
func (context *AES) Load(key []byte) error {
	if len(key) != 64 {
		return errors.New("key length incorrect")
	}
	context.AESkey = key
	return nil
}

// Returns the AES key in base64.
func (context *AES) Get() []byte {
	encode := base64.StdEncoding.EncodeToString(context.AESkey)
	return []byte(encode)
}

// Encrypts the data and returns the cipher in base64.
func (context AES) Encrypt(data []byte) ([]byte, error) {
	cipher, err := aes.NewCipher(context.AESkey)
	if err != nil {
		return nil, err
	}
	output := make([]byte, len(data))
	cipher.Encrypt(output, []byte(data))
	return []byte(base64.StdEncoding.EncodeToString(output)), nil
}

// Decrypts the data and returns the plaintext.
func (context AES) Decrypt(cipher []byte) ([]byte, error) {
	cipherText, err := base64.StdEncoding.DecodeString(string(cipher))
	if err != nil {
		return nil, err
	}
	cipherKey, err := aes.NewCipher(context.AESkey)
	if err != nil {
		return nil, err
	}
	output := make([]byte, len(cipherText))
	cipherKey.Decrypt(output, cipherText)
	return output, nil
}

func EncodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func DecodeBase64(data string) ([]byte, error) {
	bytes, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func RandStr(size int) string {
	randBytes := make([]byte, size)
	rand.Read(randBytes)
	return EncodeBase64(randBytes)
}
func RandStrAlphabet(size int) string {
	chars := "abcdefghijklmnopqrstuvwxyz"
	var result string
	for i := 0; i < size; i++ {
		random.Seed(time.Now().UnixNano())
		value := random.Int()
		value %= len(chars)
		result += string(chars[value])
	}
	return result
}
func RandBytes(size int) ([]byte, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}
func NewRSA() *RSA {
	return new(RSA)
}

func NewAES() *AES {
	return new(AES)
}
