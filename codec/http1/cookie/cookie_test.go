package cookie

import (
	"errors"
	"testing"
	"time"
)

func TestDecodeCookieHeaderParsesPairs(t *testing.T) {
	cookies, err := DecodeCookieHeader("Cookie: sid=abc; theme=dark")
	if err != nil {
		t.Fatal(err)
	}
	if len(cookies) != 2 {
		t.Fatalf("cookies=%d, want 2", len(cookies))
	}
	if cookies[0].Name != "sid" || cookies[0].Value != "abc" {
		t.Fatalf("first cookie=%+v", cookies[0])
	}
	if cookies[1].Name != "theme" || cookies[1].Value != "dark" {
		t.Fatalf("second cookie=%+v", cookies[1])
	}
}

func TestDecodeSetCookieParsesAttributes(t *testing.T) {
	c, err := DecodeSetCookie("Set-Cookie: sid=abc; Path=/; Domain=example.com; Max-Age=60; Expires=Wed, 21 Oct 2015 07:28:00 GMT; Secure; HttpOnly; SameSite=Lax; Partitioned")
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "sid" || c.Value != "abc" || c.Path != "/" || c.Domain != "example.com" {
		t.Fatalf("cookie=%+v", c)
	}
	if !c.HasMaxAge || c.MaxAge != 60 || c.Expires.IsZero() {
		t.Fatalf("age/expires=%+v", c)
	}
	if !c.Secure || !c.HTTPOnly || c.SameSite != SameSiteLax || !c.Partitioned {
		t.Fatalf("flags=%+v", c)
	}
}

func TestDecodeSetCookieRejectsInvalidSameSite(t *testing.T) {
	_, err := DecodeSetCookie("sid=abc; SameSite=bad")
	if !errors.Is(err, ErrInvalidSameSite) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidSameSite)
	}
}

func TestEncodeCookieHeaderUsesStableOrder(t *testing.T) {
	got, err := EncodeCookieHeader([]Cookie{
		{Name: "sid", Value: "abc"},
		{Name: "theme", Value: "dark"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "sid=abc; theme=dark" {
		t.Fatalf("header=%q", got)
	}
}

func TestEncodeSetCookieWritesAttributes(t *testing.T) {
	expires := time.Date(2015, 10, 21, 7, 28, 0, 0, time.UTC)
	got, err := EncodeSetCookie(Cookie{
		Name:        "sid",
		Value:       "abc",
		Path:        "/",
		Domain:      "example.com",
		Expires:     expires,
		MaxAge:      60,
		HasMaxAge:   true,
		Secure:      true,
		HTTPOnly:    true,
		SameSite:    SameSiteNone,
		Partitioned: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "sid=abc; Path=/; Domain=example.com; Max-Age=60; Expires=Wed, 21 Oct 2015 07:28:00 GMT; Secure; HttpOnly; SameSite=None; Partitioned"
	if got != want {
		t.Fatalf("set-cookie=%q, want %q", got, want)
	}
}

func TestEncodeRejectsInvalidName(t *testing.T) {
	_, err := EncodeSetCookie(Cookie{Name: "bad name", Value: "v"})
	if !errors.Is(err, ErrInvalidCookie) {
		t.Fatalf("err=%v, want %v", err, ErrInvalidCookie)
	}
}
