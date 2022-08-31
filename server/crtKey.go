package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
)

func keyGen(keyAlg string) (privKey interface{}, err error) {
	switch keyAlg {
	case "Ed25519":
		_, privKey, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
	case "RSA2048":
		privKey, err = rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
	case "RSA3072":
		privKey, err = rsa.GenerateKey(rand.Reader, 3072)
		if err != nil {
			return nil, err
		}
	case "RSA4096":
		privKey, err = rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported key algorithm")
	}
	return privKey, nil
}

func keyPairLoad(certFile, keyFile string, keyPassword []byte) (cert *x509.Certificate, key interface{}, err error) {
	file, err := os.ReadFile(certFile)
	if err != nil {
		return nil, nil, err
	}
	block, _ := pem.Decode(file)

	cert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, nil, err
	}
	file, err = os.ReadFile(keyFile)
	if err != nil {
		return nil, nil, err
	}
	block, _ = pem.Decode(file)

	blockDecrypted, err := x509.DecryptPEMBlock(block, keyPassword)
	if err != nil {
		return nil, nil, err
	}
	key, err = x509.ParsePKCS8PrivateKey(blockDecrypted)
	if err != nil {
		return nil, nil, err
	}
	return
}

func publicKey(priv interface{}) interface{} {
	switch k := priv.(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public().(ed25519.PublicKey)
	default:
		return nil
	}
}
