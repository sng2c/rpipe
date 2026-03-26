package secure

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	log "github.com/sirupsen/logrus"
	"github.com/sng2c/rpipe/msgspec"
	"io"
	"strings"
)

var ExpireError = errors.New("SymKey has expired")

type Cryptor struct {
	PrivateKey *rsa.PrivateKey
	rdb        *redis.Client
	cache      map[string]*SymKey
}
type SymKey struct {
	Key []byte
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
func (c *Cryptor) ResetInboundSymkey(ctx context.Context, msg *msgspec.RpipeMsg) error {
	log.Debugf("Expire SYMKEY for %s\n", msg.SymkeyName())
	delete(c.cache, msg.SymkeyName())

	// 반대쪽 symm 을 다시 말아준다.`
	msgrev := msg.NewReturnMsg()
	_, err := c.RegisterNewOutboundSymkey(ctx, msgrev)
	log.Debugf("Register SYMKEY for %s\n", msgrev.SymkeyName())
	if err != nil {
		return err
	}
	return nil
}
func (c *Cryptor) RegisterPubkey(ctx context.Context, chnName string) error {
	log.Debugln("RegisterPubkey")
	{
		pubkeyStr := EncodePubkey(&c.PrivateKey.PublicKey)
		_, err := c.rdb.Set(ctx, "RPIPE:PUBKEYS:"+chnName, pubkeyStr, 0).Result()
		if err != nil {
			return err
		}
	}

	resetTargets := make(map[string]bool)

	// Delete Symkeys from ME
	{
		keys, err := c.rdb.Keys(ctx, "RPIPE:SYMKEYS:"+chnName+":*").Result()
		log.Debugf("Remove Keys %v\n", keys)
		if err != nil {
			return err
		}
		for _, k := range keys {
			_, err := c.rdb.Del(ctx, k).Result()
			if err != nil {
				log.Warningln("Failed to delete SYMKEYS: "+k, err)
				return err
			}
			ks := strings.SplitN(k, ":", 4)
			targetChnName := ks[3]
			resetTargets[targetChnName] = true
		}
	}

	// Publish Reset Symkeys to ME
	{
		keys, err := c.rdb.Keys(ctx, "RPIPE:SYMKEYS:*:"+chnName).Result()
		log.Debugln("KEYS RPIPE:SYMKEYS:*:" + chnName + "\n")
		log.Debugf("Publish Reset SYMKEYS %v\n", keys)
		if err != nil {
			return err
		}
		for _, k := range keys {
			ks := strings.SplitN(k, ":", 4)
			targetChnName := ks[2]
			resetTargets[targetChnName] = true
		}
	}

	for targetChnName := range resetTargets {
		resetMsg := msgspec.RpipeMsg{From: chnName, To: targetChnName, Control: 1}
		resetMsgJson := resetMsg.Marshal()
		log.Debugf("[PUB-%s] %s", targetChnName, resetMsgJson)
		_, err := c.rdb.Publish(ctx, targetChnName, resetMsgJson).Result()
		if err != nil {
			log.Warningln("Failed to publish SYMKEYS reset to "+targetChnName, err)
			return err
		}
	}

	return nil
}
func (c *Cryptor) FetchTargetPubkey(ctx context.Context, msg *msgspec.RpipeMsg) (*rsa.PublicKey, error) {
	result, err := c.rdb.Get(ctx, "RPIPE:PUBKEYS:"+msg.To).Result()
	if err != nil {
		return nil, err
	}
	return DecodePubkey(result), nil
}

func (c *Cryptor) FetchSymkey(ctx context.Context, msg *msgspec.RpipeMsg) (*SymKey, error) {
	symKey, ok := c.cache[msg.SymkeyName()]
	if !ok {
		symkeyFullname := fmt.Sprintf("RPIPE:SYMKEYS:%s", msg.SymkeyName())
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

func (c *Cryptor) RegisterNewOutboundSymkey(ctx context.Context, msg *msgspec.RpipeMsg) (*SymKey, error) {
	symkeyFullname := fmt.Sprintf("RPIPE:SYMKEYS:%s", msg.SymkeyName())
	pubkey, err := c.FetchTargetPubkey(ctx, msg)
	if err != nil {
		return nil, err
	}
	newKey := randStringBytes(32)
	_symkeyStr, err := EncryptPKI(pubkey, newKey)
	if err != nil {
		return nil, err
	}
	_, err = c.rdb.Set(ctx, symkeyFullname, _symkeyStr, 0).Result()
	if err != nil {
		return nil, err
	}
	symkey := SymKey{
		Key: newKey,
	}
	c.cache[msg.SymkeyName()] = &symkey
	return &symkey, nil
}

func EncryptMessage(symKey *SymKey, message []byte) ([]byte, error) {
	block, err := aes.NewCipher(symKey.Key)
	if err != nil {
		return nil, fmt.Errorf("could not create cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("could not create GCM: %v", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("could not generate nonce: %v", err)
	}
	return gcm.Seal(nonce, nonce, message, nil), nil
}

func DecryptMessage(symKey *SymKey, cipherText []byte) ([]byte, error) {
	block, err := aes.NewCipher(symKey.Key)
	if err != nil {
		return nil, fmt.Errorf("could not create cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("could not create GCM: %v", err)
	}
	nonceSize := gcm.NonceSize()
	if len(cipherText) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, cipherText := cipherText[:nonceSize], cipherText[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return nil, fmt.Errorf("could not decrypt: %v", err)
	}
	return plaintext, nil
}

func EncryptPKI(publicKey *rsa.PublicKey, s []byte) (string, error) {
	ciphertext, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, publicKey, s, nil)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}
func DecryptPKI(privateKey *rsa.PrivateKey, s string) ([]byte, error) {
	plain, _ := base64.StdEncoding.DecodeString(s)
	decrypted, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, privateKey, plain, nil)
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
