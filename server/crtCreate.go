package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

func crtCreate(action string, serverName string) {
	if len(os.Args) <= 2 {
		os.Stderr.WriteString("Create error: Missing object\n")
		return
	}
	if _, err := os.Stat("./certs/ca/ca.crt"); !os.IsNotExist(err) {
		log.Fatalln("CA certificate already exists")
	}
	if _, err := os.Stat("./certs/server/server.crt"); !os.IsNotExist(err) {
		log.Fatalln("Server certificate already exists")
	}
	if _, err := os.Stat("./certs/client/client.crt"); !os.IsNotExist(err) {
		log.Fatalln("Client certificate already exists")
	}

	switch strings.ToLower(os.Args[2]) {
	case "ca":
		password, err := readPasswordConfirm()
		if err != nil {
			os.Stderr.WriteString(fmt.Sprintf("error: %s\n", err.Error()))
			return
		}
		log.Println("generation in progress")
		if err := crtCreateCA("RSA4096", 240, password); err != nil {
			log.Fatalln(err.Error())
		}
	case "server":
		dnsNames, ipAddresses, err := crtGetSettings(serverName)
		if err != nil {
			log.Fatalln(err.Error())
		}
		if err := crtCreateServer("RSA3072", 240, dnsNames, ipAddresses, readPassword()); err != nil {
			log.Fatalln(err.Error())
		}
	case "client":
		dnsNames, ipAddresses, err := crtGetSettings(serverName)
		if err != nil {
			log.Fatalln(err.Error())
		}
		if err := crtCreateClient("RSA3072", 240, dnsNames, ipAddresses, readPassword()); err != nil {
			log.Fatalln(err.Error())
		}
	case "all":
		fmt.Println("Generating certificates...")
		dnsNames, ipAddresses, err := crtGetSettings(serverName)
		if err != nil {
			log.Fatalln(err.Error())
		}
		password := make([]byte, 26)
		rand.Read(password)
		if err := crtCreateCA("RSA4096", 240, password); err != nil {
			log.Fatalln(err.Error())
		}
		if err := crtCreateServer("RSA3072", 240, dnsNames, ipAddresses, password); err != nil {
			log.Fatalln(err.Error())
		}
		if err := crtCreateClient("RSA3072", 240, dnsNames, ipAddresses, password); err != nil {
			log.Fatalln(err.Error())
		}
	default:
		os.Stderr.WriteString("Get error: Unknown object\n")
		return
	}
}

func crtGetSettings(serverName string) (dnsNames []string, ipAddresses []net.IP, err error) {
	if serverName == "" {
		return nil, nil, errors.New("missing server name")
	}
	dnsNames = append(dnsNames, serverName)
	addrs, err := net.LookupHost(dnsNames[0])
	if err != nil {
		return nil, nil, fmt.Errorf("DNS lookup failed: %s", err.Error())
	}
	for i := range addrs {
		if ip := net.ParseIP(addrs[i]); ip != nil {
			ipAddresses = append(ipAddresses, ip)
		}
	}
	if len(ipAddresses) == 0 {
		return nil, nil, errors.New("no IP addresses for selected server name")
	}
	return dnsNames, ipAddresses, nil
}

func readPasswordConfirm() ([]byte, error) {
	fmt.Print("Enter password: ")
	p1, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return nil, errors.New("read password")
	}
	fmt.Print("\nConfirm password: ")
	p2, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return nil, errors.New("read password")
	}
	fmt.Print("\n")
	if len(string(p1)) < 4 {
		return nil, errors.New("password must have 4 or more chars")
	}
	if string(p1) != string(p2) {
		return nil, errors.New("password mismatch")
	}
	return p1, nil
}

func readPassword() []byte {
	fmt.Print("Enter password: ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return nil
	}
	fmt.Println("")
	return password
}

func crtCreateCA(keyAlg string, validMonths int, password []byte) error {
	// Create private key
	privKey, err := keyGen(keyAlg)
	if err != nil {
		return fmt.Errorf("private key generation failed: %s", err.Error())
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %s", err.Error())
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"gscheduler"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, validMonths, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	// Only RSA subject keys should have the KeyEncipherment KeyUsage bits set. In
	// the context of TLS this KeyUsage is particular to RSA key exchange and
	// authentication.
	if _, isRSA := privKey.(*rsa.PrivateKey); isRSA {
		template.KeyUsage |= x509.KeyUsageKeyEncipherment
	}

	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, publicKey(privKey), privKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %s", err.Error())
	}

	// Write files
	os.MkdirAll(filepath.Join(filepath.Dir(os.Args[0]), "certs", "ca"), 0700)
	caCrtFileName := filepath.Join(filepath.Dir(os.Args[0]), "certs", "ca", "ca.crt")
	certOut, err := os.OpenFile(caCrtFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", caCrtFileName, err.Error())
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write data to %s: %s", caCrtFileName, err.Error())
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %s", caCrtFileName, err.Error())
	}
	fmt.Printf("File %s created\n", caCrtFileName)
	caKeyFileName := filepath.Join(filepath.Dir(os.Args[0]), "certs", "ca", "ca.key")
	keyOut, err := os.OpenFile(caKeyFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", caKeyFileName, err.Error())
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %s", err.Error())
	}
	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	}

	pemBlock, err = x509.EncryptPEMBlock(rand.Reader, pemBlock.Type, pemBlock.Bytes, password, x509.PEMCipherAES256)
	if err != nil {
		return fmt.Errorf("failed to encrypt private key: %s", err.Error())
	}
	keyOut.WriteString(string(pem.EncodeToMemory(pemBlock)))
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %s", caKeyFileName, err.Error())
	}
	fmt.Printf("File %s created\n", caKeyFileName)
	return nil
}

func crtCreateServer(keyAlg string, validMonths int, dnsNames []string, ipAddresses []net.IP, password []byte) error {
	// Create private key
	privKey, err := keyGen(keyAlg)
	if err != nil {
		return fmt.Errorf("private key generation failed: %s", err.Error())
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %s", err.Error())
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   dnsNames[0],
			Organization: []string{"gscheduler"},
		},
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, validMonths, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	// Only RSA subject keys should have the KeyEncipherment KeyUsage bits set. In
	// the context of TLS this KeyUsage is particular to RSA key exchange and
	// authentication.
	if _, isRSA := privKey.(*rsa.PrivateKey); isRSA {
		template.KeyUsage |= x509.KeyUsageKeyEncipherment
	}

	// Load CA certificte + key
	caCert, caKey, err := keyPairLoad(
		filepath.Join(filepath.Dir(os.Args[0]), "certs", "ca", "ca.crt"),
		filepath.Join(filepath.Dir(os.Args[0]), "certs", "ca", "ca.key"),
		password)
	if err != nil {
		return fmt.Errorf("failed to load CA key pair: %s", err.Error())
	}
	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, publicKey(privKey), caKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %s", err.Error())
	}

	// Write files
	os.MkdirAll(filepath.Join(filepath.Dir(os.Args[0]), "certs", "server"), 0700)
	certFileName := filepath.Join(filepath.Dir(os.Args[0]), "certs", "server", "server.crt")
	certOut, err := os.OpenFile(certFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", certFileName, err.Error())
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write data to %s: %s", certFileName, err.Error())
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %s", certFileName, err.Error())
	}
	fmt.Printf("File %s created\n", certFileName)

	certKeyFileName := filepath.Join(filepath.Dir(os.Args[0]), "certs", "server", "server.key")
	keyOut, err := os.OpenFile(certKeyFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", certKeyFileName, err.Error())
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %s", err.Error())
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return fmt.Errorf("failed to write data to %s: %s", certKeyFileName, err.Error())
	}
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %s", certKeyFileName, err.Error())
	}
	fmt.Printf("File %s created\n", certKeyFileName)
	return nil
}

func crtCreateClient(keyAlg string, validMonths int, dnsNames []string, ipAddresses []net.IP, password []byte) error {
	// Create private key
	privKey, err := keyGen(keyAlg)
	if err != nil {
		return fmt.Errorf("private key generation failed: %s", err.Error())
	}

	// Create certificate template
	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return fmt.Errorf("failed to generate serial number: %s", err.Error())
	}
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   dnsNames[0],
			Organization: []string{"gscheduler"},
		},
		DNSNames:              dnsNames,
		IPAddresses:           ipAddresses,
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, validMonths, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	// Only RSA subject keys should have the KeyEncipherment KeyUsage bits set. In
	// the context of TLS this KeyUsage is particular to RSA key exchange and
	// authentication.
	if _, isRSA := privKey.(*rsa.PrivateKey); isRSA {
		template.KeyUsage |= x509.KeyUsageKeyEncipherment
	}
	// Load CA certificte + key
	caCert, caKey, err := keyPairLoad(
		filepath.Join(filepath.Dir(os.Args[0]), "certs", "ca", "ca.crt"),
		filepath.Join(filepath.Dir(os.Args[0]), "certs", "ca", "ca.key"),
		password)
	if err != nil {
		return fmt.Errorf("failed to load CA key pair: %s", err.Error())
	}
	// Create certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, publicKey(privKey), caKey)
	if err != nil {
		return fmt.Errorf("failed to create certificate: %s", err.Error())
	}
	os.MkdirAll(filepath.Join(filepath.Dir(os.Args[0]), "certs", "client"), 0700)
	certFileName := filepath.Join(filepath.Dir(os.Args[0]), "certs", "client", "client.crt")
	certOut, err := os.OpenFile(certFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", certFileName, err.Error())
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to write data to %s: %s", certFileName, err.Error())
	}
	if err := certOut.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %s", certFileName, err.Error())
	}
	fmt.Printf("File %s created\n", certFileName)
	certKeyFileName := filepath.Join(filepath.Dir(os.Args[0]), "certs", "client", "client.key")
	keyOut, err := os.OpenFile(certKeyFileName, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s for writing: %s", certKeyFileName, err.Error())
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return fmt.Errorf("failed to marshal private key: %s", err.Error())
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "PRIVATE KEY", Bytes: privBytes}); err != nil {
		return fmt.Errorf("failed to write data to %s: %s", certKeyFileName, err.Error())
	}
	if err := keyOut.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %s", certKeyFileName, err.Error())
	}
	fmt.Printf("File %s created\n", certKeyFileName)
	return nil
}
