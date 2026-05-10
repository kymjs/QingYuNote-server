package appleid

import "testing"

func TestInspectIdentityToken_RS256Sample(t *testing.T) {
	// ParseUnverified 不校验签名；三段结构与真实 JWT 一致（alg=RS256）。
	raw := "eyJhbGciOiJSUzI1NiIsImtpZCI6IkFCQyJ9.eyJpc3MiOiJodHRwczovL2FwcGxlaWQuYXBwbGUuY29tIiwiYXVkIjoiY29tLmt5bWpzLm5vdGUiLCJzdWIiOiIwMDEyMzQifQ.sig"
	p := InspectIdentityToken(raw)
	if p.Alg != "RS256" {
		t.Fatalf("alg=%q want RS256", p.Alg)
	}
	if p.Kid != "ABC" {
		t.Fatalf("kid=%q want ABC", p.Kid)
	}
	if p.Iss != "https://appleid.apple.com" {
		t.Fatalf("iss=%q", p.Iss)
	}
	if p.Aud != "com.kymjs.note" {
		t.Fatalf("aud=%q", p.Aud)
	}
	if p.SubPrefix != "001234" {
		t.Fatalf("sub_prefix=%q", p.SubPrefix)
	}
	if p.TokenLen != len(raw) || p.NumParts != 3 {
		t.Fatalf("meta token_len=%d parts=%d", p.TokenLen, p.NumParts)
	}
}
