package bedrock

import (
	"encoding/hex"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestDeriveSigningKey checks the key derivation against the example published
// in the AWS Signature Version 4 documentation.
func TestDeriveSigningKey(t *testing.T) {
	key := deriveSigningKey("wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "20120215", "us-east-1", "iam")

	const want = "f4780e2d9f65fa895f9c67b32ce1baf0b0d8a43505a000a1a9e090d414db404d"
	if got := hex.EncodeToString(key); got != want {
		t.Errorf("signing key = %s, want %s", got, want)
	}
}

// TestSignRequest pins the complete signature of a Bedrock Converse request
// against a signature computed with an independent implementation.
func TestSignRequest(t *testing.T) {
	body := []byte(`{"a":1}`)

	req, err := http.NewRequest(
		http.MethodPost,
		"https://bedrock-runtime.eu-north-1.amazonaws.com/model/test-model/converse",
		strings.NewReader(string(body)),
	)
	if err != nil {
		t.Fatalf("failed to build the request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	signedAt := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	credentials := Credentials{
		AccessKeyID:     "AKIDEXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}

	if err := signRequest(req, body, credentials, "eu-north-1", ServiceName, signedAt); err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	authorization := req.Header.Get("Authorization")

	const wantCredential = "Credential=AKIDEXAMPLE/20150830/eu-north-1/bedrock/aws4_request"
	if !strings.Contains(authorization, wantCredential) {
		t.Errorf("Authorization = %q, want it to contain %q", authorization, wantCredential)
	}

	const wantSignedHeaders = "SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date"
	if !strings.Contains(authorization, wantSignedHeaders) {
		t.Errorf("Authorization = %q, want it to contain %q", authorization, wantSignedHeaders)
	}

	const wantSignature = "Signature=1c659602aa6a0226c963e68d148057bc10ea8597cc94ec070f29ad9ff6706c15"
	if !strings.Contains(authorization, wantSignature) {
		t.Errorf("Authorization = %q, want it to contain %q", authorization, wantSignature)
	}

	if req.Header.Get("X-Amz-Date") != "20150830T123600Z" {
		t.Errorf("X-Amz-Date = %q", req.Header.Get("X-Amz-Date"))
	}
	const wantPayloadHash = "015abd7f5cc57a2dd94b7590f04ad8084273905ee33ec5cebeae62276a97f862"
	if req.Header.Get("X-Amz-Content-Sha256") != wantPayloadHash {
		t.Errorf("X-Amz-Content-Sha256 = %q, want %s", req.Header.Get("X-Amz-Content-Sha256"), wantPayloadHash)
	}
}

// TestSignRequestIncludesSessionToken checks temporary credential support.
func TestSignRequestIncludesSessionToken(t *testing.T) {
	body := []byte(`{}`)
	req, err := http.NewRequest(http.MethodPost, "https://bedrock-runtime.us-east-1.amazonaws.com/model/m/converse", nil)
	if err != nil {
		t.Fatalf("failed to build the request: %v", err)
	}

	credentials := Credentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", SessionToken: "token-123"}
	if err := signRequest(req, body, credentials, "us-east-1", ServiceName, time.Now()); err != nil {
		t.Fatalf("signing failed: %v", err)
	}

	if req.Header.Get("X-Amz-Security-Token") != "token-123" {
		t.Error("expected the session token to be sent")
	}
	if !strings.Contains(req.Header.Get("Authorization"), "x-amz-security-token") {
		t.Error("expected the session token to be part of the signed headers")
	}
}

// TestSignRequestRequiresCredentials checks the guard against empty credentials.
func TestSignRequestRequiresCredentials(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://bedrock-runtime.us-east-1.amazonaws.com/", nil)
	if err != nil {
		t.Fatalf("failed to build the request: %v", err)
	}

	if err := signRequest(req, nil, Credentials{}, "us-east-1", ServiceName, time.Now()); err == nil {
		t.Error("expected an error when credentials are missing")
	}
}
