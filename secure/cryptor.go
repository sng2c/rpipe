package secure

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
	"github.com/sng2c/rpipe/protocol"
	"io"
	"strings"
	"time"
)

const ExpireDuration = 30 * time.Minute

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
func (c *Cryptor) ResetInboundSymkey(ctx context.Context, msg *protocol.Msg) error {
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

	{
		pubkeyStr := EncodePubkey(&c.PrivateKey.PublicKey)
		_, err := c.rdb.Set(ctx, "RPIPE:PUBKEYS:"+chnName, pubkeyStr, 0).Result()
		if err != nil {
			return err
		}
	}

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
				log.Warningln("Delete SYMKEYS "+k, err)
				return err
			}
		}
	}

	// Publish Reset Symkeys to ME
	{
		keys, err := c.rdb.Keys(ctx, "RPIPE:SYMKEYS:*:"+chnName).Result()
		log.Debugf("Publish Reset SYMKEYS %v\n", keys)
		if err != nil {
			return err
		}
		for _, k := range keys {
			ks := strings.SplitN(k, ":", 4)
			targetChnName := ks[2]
			resetMsg := protocol.Msg{From: chnName, To: targetChnName, Control: 1}
			resetMsgJson := resetMsg.Marshal()
			log.Debugf("[PUB-%s] %s", targetChnName, resetMsgJson)
			_, err := c.rdb.Publish(ctx, targetChnName, resetMsgJson).Result()
			if err != nil {
				log.Warningln("Publish Reset SYMKEYS to "+targetChnName, err)
				return err
			}
		}
	}

	return nil
}
func (c *Cryptor) FetchTargetPubkey(ctx context.Context, msg *protocol.Msg) (*rsa.PublicKey, error) {
	result, err := c.rdb.Get(ctx, "RPIPE:PUBKEYS:"+msg.To).Result()
	if err != nil {
		return nil, err
	}
	return DecodePubkey(result), nil
}

func (c *Cryptor) FetchSymkey(ctx context.Context, msg *protocol.Msg) (*SymKey, error) {
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

func (c *Cryptor) RegisterNewOutboundSymkey(ctx context.Context, msg *protocol.Msg) (*SymKey, error) {
	symkeyFullname := fmt.Sprintf("RPIPE:SYMKEYS:%s", msg.SymkeyName())
	pubkey, err := c.FetchTargetPubkey(ctx, msg)
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
		Key: newKey,
	}
	c.cache[msg.SymkeyName()] = &symkey
	return &symkey, nil
}

func EncryptMessage(symKey *SymKey, message []byte) ([]byte, error) {
	byteMsg := message
	block, err := aes.NewCipher(symKey.Key)
	if err != nil {
		return nil, fmt.Errorf("could not create new cipher: %v", err)
	}

	cipherText := make([]byte, aes.BlockSize+len(byteMsg))
	iv := cipherText[:aes.BlockSize]
	if _, err = io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("could not encrypt: %v", err)
	}

	stream := cipher.NewCFBEncrypter(block, iv)
	stream.XORKeyStream(cipherText[aes.BlockSize:], byteMsg)

	return cipherText, nil
}
func DecryptMessage(symKey *SymKey, cipherText []byte) ([]byte, error) {
	block, err := aes.NewCipher(symKey.Key)
	if err != nil {
		return nil, fmt.Errorf("could not create new cipher: %v", err)
	}

	if len(cipherText) < aes.BlockSize {
		return nil, fmt.Errorf("invalid ciphertext block size")
	}

	iv := cipherText[:aes.BlockSize]
	cipherText = cipherText[aes.BlockSize:]

	stream := cipher.NewCFBDecrypter(block, iv)
	stream.XORKeyStream(cipherText, cipherText)

	return cipherText, nil
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
