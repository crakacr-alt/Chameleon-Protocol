package mobile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// EnsureInspectorCA creates (once) a local CA used only when the user
// explicitly enables HTTPS inspection and installs the certificate through
// the Android system certificate installer. The private key never leaves the
// application state directory.
func EnsureInspectorCA(stateDir string) string {
	if stateDir == "" {
		return "ERROR: state directory is required"
	}
	dir := filepath.Join(stateDir, "inspector")
	certPath := filepath.Join(dir, "inspector-ca.crt")
	keyPath := filepath.Join(dir, "inspector-ca.key")

	if der, err := readCertificateDER(certPath); err == nil {
		return base64.StdEncoding.EncodeToString(der)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "ERROR: " + err.Error()
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "ERROR: generate inspector CA key: " + err.Error()
	}
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return "ERROR: generate inspector CA serial: " + err.Error()
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Chameleon Inspector Local CA",
			Organization: []string{"Chameleon Protocol"},
		},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "ERROR: create inspector CA: " + err.Error()
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "ERROR: encode inspector CA key: " + err.Error()
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certPath, certPEM, 0o600); err != nil {
		return "ERROR: write inspector CA certificate: " + err.Error()
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return "ERROR: write inspector CA private key: " + err.Error()
	}
	return base64.StdEncoding.EncodeToString(der)
}

func InspectorCAFingerprint(stateDir string) string {
	der, err := readCertificateDER(filepath.Join(stateDir, "inspector", "inspector-ca.crt"))
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

func readCertificateDER(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("invalid inspector CA certificate")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return nil, err
	}
	return block.Bytes, nil
}
