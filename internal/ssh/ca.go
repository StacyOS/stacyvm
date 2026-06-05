package ssh

import (
	"crypto/rand"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// Certificate extension keys carrying StacyVM identity inside a user cert.
const (
	certExtOwner   = "stacyvm-owner@stacyvm.dev"
	certExtTenant  = "stacyvm-tenant@stacyvm.dev"
	certExtSubject = "stacyvm-subject@stacyvm.dev"
)

// SignUserCertificate mints a short-lived OpenSSH user certificate that
// authorizes userKey to connect as the given sandbox (the SSH username), bound
// to identity. It is signed by the deployment's User CA (caSigner). The CLI
// uses this for the smooth `stacy ssh` flow without long-lived registered keys.
func SignUserCertificate(caSigner gossh.Signer, userKey gossh.PublicKey, identity Identity, sandboxID string, ttl time.Duration) (*gossh.Certificate, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	now := time.Now()
	cert := &gossh.Certificate{
		Key:             userKey,
		Serial:          uint64(now.UnixNano()),
		CertType:        gossh.UserCert,
		KeyId:           identity.Subject,
		ValidPrincipals: []string{sandboxID},
		ValidAfter:      uint64(now.Add(-1 * time.Minute).Unix()),
		ValidBefore:     uint64(now.Add(ttl).Unix()),
		Permissions: gossh.Permissions{
			Extensions: map[string]string{
				certExtOwner:   identity.OwnerID,
				certExtTenant:  identity.TenantID,
				certExtSubject: identity.Subject,
				"permit-pty":   "",
			},
		},
	}
	if err := cert.SignCert(rand.Reader, caSigner); err != nil {
		return nil, err
	}
	return cert, nil
}

func identityFromCert(cert *gossh.Certificate) Identity {
	ext := cert.Permissions.Extensions
	return Identity{
		Subject:  ext[certExtSubject],
		OwnerID:  ext[certExtOwner],
		TenantID: ext[certExtTenant],
	}
}
