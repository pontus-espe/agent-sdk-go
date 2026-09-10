package bedrock

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Credentials are the AWS credentials used to sign requests.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// signRequest signs an HTTP request with AWS Signature Version 4.
//
// The Bedrock runtime only needs this small subset of SigV4, which keeps the
// SDK free of third party dependencies.
func signRequest(req *http.Request, payload []byte, creds Credentials, region, service string, now time.Time) error {
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return fmt.Errorf("missing AWS credentials")
	}

	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")

	payloadHash := sha256Hex(payload)

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}

	canonicalHeaders, signedHeaders := canonicalizeHeaders(req)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req),
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := strings.Join([]string{dateStamp, region, service, "aws4_request"}, "/")
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(creds.SecretAccessKey, dateStamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credentialScope, signedHeaders, signature,
	))

	return nil
}

// canonicalURI returns the path used in the canonical request.
func canonicalURI(req *http.Request) string {
	path := req.URL.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

// canonicalizeHeaders builds the canonical header block and the signed header
// list. Only the headers required by SigV4 are included.
func canonicalizeHeaders(req *http.Request) (string, string) {
	headers := map[string]string{
		"host": req.URL.Host,
	}

	for _, name := range []string{"Content-Type", "X-Amz-Date", "X-Amz-Security-Token", "X-Amz-Content-Sha256", "X-Amzn-Bedrock-Accept"} {
		if value := req.Header.Get(name); value != "" {
			headers[strings.ToLower(name)] = strings.TrimSpace(value)
		}
	}

	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)

	var canonical strings.Builder
	for _, name := range names {
		canonical.WriteString(name)
		canonical.WriteString(":")
		canonical.WriteString(headers[name])
		canonical.WriteString("\n")
	}

	return canonical.String(), strings.Join(names, ";")
}

// deriveSigningKey derives the SigV4 signing key.
func deriveSigningKey(secret, dateStamp, region, service string) []byte {
	date := hmacSHA256([]byte("AWS4"+secret), []byte(dateStamp))
	regionKey := hmacSHA256(date, []byte(region))
	serviceKey := hmacSHA256(regionKey, []byte(service))
	return hmacSHA256(serviceKey, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
