package messages

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
	"io"
	"time"
)

const ExpireDuration = 30 * time.Minute

var ExpireError = errors.New("SymKey has expired")

type Cryptor struct {
	PrivateKey *rsa.PrivateKey
	rdb   *redis.Client
	cache map[string]*SymKey
}
type SymKey struct {
	Key      []byte
}

func NewCryptor(rdb *redis.Client) *Cryptor {
	privateKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	// clear SYMKEYS
	return &Cryptor{
		PrivateKey: privateKey,
		rdb:        rdb,
		cache:      make(map[string]*SymKey),
	}
}

func (c *Cryptor) RegisterPubkey(ctx context.Context, chnName string) error {

	keys, err := c.rdb.Keys(ctx, "RPIPE:SYMKEYS:"+chnName+":*").Result()
	log.Debugf("Remove Keys %v\n", keys)
	if err != nil {
		return err
	}
	for _, k := range keys {
		_, err := c.rdb.Del(ctx, k).Result()
		if err != nil {
			log.Debugln("Delete SYMKEYS "+k, err)
			return err
		}
	}

	pubkeyStr := EncodePubkey(&c.PrivateKey.PublicKey)
	_, err = c.rdb.Set(ctx, "RPIPE:PUBKEYS:"+chnName, pubkeyStr, 0).Result()
	if err != nil {
		return err
	}

	return nil
}
func (c *Cryptor) FetchRemotePubkey(ctx context.Context, msg *Msg) (*rsa.PublicKey, error) {
	result, err := c.rdb.Get(ctx, "RPIPE:PUBKEYS:"+msg.To).Result()
	if err != nil {
		return nil, err
	}
	return DecodePubkey(result), nil
}

func (c *Cryptor) FetchRemoteSymkey(ctx context.Context, msg *Msg) (*SymKey, error) {
	symkeyFullname := fmt.Sprintf("RPIPE:SYMKEYS:%s", msg.SymkeyName())
	if msg.Refresh {
		log.Debugf("Refresh symKey for %s\n", symkeyFullname)
		// 새로 가져온다.
		delete(c.cache, msg.SymkeyName())
		// symm 을 다시 말아준다.
		_, err := c.RegisterNewSymkeyForRemote(ctx, &Msg{From: msg.To, To:msg.From})
		if err != nil {
			return nil, err
		}
	}
	symKey, ok := c.cache[msg.SymkeyName()]
	if !ok {

		log.Debugf("Update Symkey %s\n", symkeyFullname)
		pkiCryptedSymkey, err := c.rdb.Get(ctx, symkeyFullname).Result()
		if err != nil {
			return nil, ExpireError
		}
		_key, err := DecryptPKI(c.PrivateKey, pkiCryptedSymkey)
		if err != nil {
			return nil, err
		}
		symKey = &SymKey{Key: _key}
		c.cache[msg.SymkeyName()] = symKey
	}
	return symKey, nil
}

func randStringBytes(n int) []byte {
	key := make([]byte, n)
	_, err := rand.Read(key)
	if err != nil {
		return nil
	}
	return key
}

func (c *Cryptor) RegisterNewSymkeyForRemote(ctx context.Context, msg *Msg) (*SymKey, error) {
	symkeyFullname := fmt.Sprintf("RPIPE:SYMKEYS:%s", msg.SymkeyName())
	pubkey, err := c.FetchRemotePubkey(ctx, msg)
	if err != nil {
		return nil, err
	}
	newKey := randStringBytes(16)
	_symkeyStr, err := EncryptPKI(pubkey, newKey)
	if err != nil {
		return nil, err
	}
	_, err = c.rdb.Set(ctx, symkeyFullname, _symkeyStr, 0).Result()
	if err != nil {
		return nil, err
	}
	symkey := SymKey{
		Key:      newKey,
	}
	c.cache[msg.SymkeyName()] = &symkey
	return &symkey, nil
}
func EncryptMessage(symKey *SymKey, message string) (string, error) {
	byteMsg := []byte(message)
	block, err := aes.NewCipher(symKey.Key)
	if err != nil {
		return "", fmt.Errorf("could not create new cipher: %v", err)
	}

	cipherText := make([]byte, aes.BlockSize+len(byteMsg))
	iv := cipherText[:aes.BlockSize]
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("could not encrypt: %v", err)
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(cipherText[aes.BlockSize:], byteMsg)

	return base64.StdEncoding.EncodeToString(cipherText), nil
}
func DecryptMessage(symKey *SymKey, message string) (string, error) {
	cipherText, err := base64.StdEncoding.DecodeString(message)
	if err != nil {
		return "", fmt.Errorf("could not base64 decode: %v", err)
	}

	block, err := aes.NewCipher(symKey.Key)
	if err != nil {
		return "", fmt.Errorf("could not create new cipher: %v", err)
	}

	if len(cipherText) < aes.BlockSize {
		return "", fmt.Errorf("invalid ciphertext block size")
	}

	iv := cipherText[:aes.BlockSize]
	cipherText = cipherText[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(cipherText, cipherText)

	return string(cipherText), nil
}

func EncryptPKI(publicKey *rsa.PublicKey, s []byte) (string, error) {
	ciphertext, err := rsa.EncryptPKCS1v15( // 평문을 공개 키로 암호화
		rand.Reader,
		publicKey, // 공개키
		s,
	)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
func DecryptPKI(privateKey *rsa.PrivateKey, s string) ([]byte, error) {
	plain, _ := base64.StdEncoding.DecodeString(s)
	decrypted, err := rsa.DecryptPKCS1v15( // 암호화된 데이터를 개인 키로 복호화
		rand.Reader,
		privateKey, // 개인키
		plain,
	)
	if err != nil {
		return nil, err
	}
	return decrypted, nil
}
func EncodePubkey(publicKey *rsa.PublicKey) string {
	x509EncodedPub, _ := x509.MarshalPKIXPublicKey(publicKey)
	pemEncodedPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509EncodedPub})
	return string(pemEncodedPub)
}
func EncodePrivkey(privateKey *rsa.PrivateKey) string {
	x509Encoded := x509.MarshalPKCS1PrivateKey(privateKey)
	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509Encoded})
	return string(pemEncoded)
}
func DecodePubkey(pemEncodedPub string) *rsa.PublicKey {
	blockPub, _ := pem.Decode([]byte(pemEncodedPub))
	x509EncodedPub := blockPub.Bytes
	genericPublicKey, _ := x509.ParsePKIXPublicKey(x509EncodedPub)
	publicKey := genericPublicKey.(*rsa.PublicKey)
	return publicKey
}
func DecodePrivkey(pemEncoded string) *rsa.PrivateKey {
	block, _ := pem.Decode([]byte(pemEncoded))
	x509Encoded := block.Bytes
	privateKey, _ := x509.ParsePKCS1PrivateKey(x509Encoded)
	return privateKey
}
