package http1

import "testing"

func TestDecodeQueryStringPreservesOrderAndRepeatedKeys(t *testing.T) {
	query, err := DecodeQueryString("/search?q=go+netty&tag=a&tag=b#top", 8)
	if err != nil {
		t.Fatal(err)
	}
	if query.Path != "/search" || query.Fragment != "top" {
		t.Fatalf("query=%+v", query)
	}
	if len(query.Params) != 3 || query.Params[0].Name != "q" || query.Params[0].Value != "go netty" {
		t.Fatalf("params=%+v", query.Params)
	}
	values := query.Values()
	if got := values["tag"]; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("values=%v", values)
	}
}

func TestAppendQueryStringEncodesParamsInCallOrder(t *testing.T) {
	got, err := AppendQueryString(nil, "/search", []QueryParam{
		{Name: "q", Value: "go netty"},
		{Name: "tag", Value: "a"},
		{Name: "tag", Value: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "/search?q=go+netty&tag=a&tag=b" {
		t.Fatalf("uri=%q", got)
	}
}

func TestDecodeQueryStringEnforcesParamLimit(t *testing.T) {
	_, err := DecodeQueryString("/x?a=1&b=2", 1)
	if err == nil {
		t.Fatal("missing param-limit error")
	}
}

func TestQueryStringValuesIsIndependent(t *testing.T) {
	query := QueryString{Params: []QueryParam{{Name: "a", Value: "1"}}}
	values := query.Values()
	values.Add("a", "2")
	if got := query.Values()["a"]; len(got) != 1 || got[0] != "1" {
		t.Fatalf("values leaked mutation: %v", got)
	}
}
