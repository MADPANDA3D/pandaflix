package providers

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestVixSrcParseAPISrc(t *testing.T) {
	src, err := vixsrcParseAPISrc([]byte(`{"src":"/embed/234272?token=abc&expires=1&lang=en"}`))
	if err != nil || !strings.HasPrefix(src, "/embed/234272") {
		t.Fatalf("unexpected src=%q err=%v", src, err)
	}
	if _, err := vixsrcParseAPISrc([]byte(`not json`)); err == nil {
		t.Fatal("invalid json accepted")
	}
	if _, err := vixsrcParseAPISrc([]byte(`{"src":""}`)); err == nil {
		t.Fatal("empty src accepted")
	}
	if _, err := vixsrcParseAPISrc([]byte(`{"src":"https://evil.example/x"}`)); err == nil {
		t.Fatal("non-embed src accepted")
	}
}

func TestVixSrcParseMasterPlaylist(t *testing.T) {
	html := []byte(`
        window.masterPlaylist = {
            params: {
                'token': '4a6bf887771fc587613b03b8d9468a6c',
                'expires': '1794365379',
                'asn': '',
            },
            url: 'https://vixsrc.to/playlist/234272',
        }
        window.canPlayFHD = true
`)
	u, token, expires, err := vixsrcParseMasterPlaylist(html)
	if err != nil {
		t.Fatal(err)
	}
	if u != "https://vixsrc.to/playlist/234272" || token != "4a6bf887771fc587613b03b8d9468a6c" || expires != "1794365379" {
		t.Fatalf("unexpected parse: %q %q %q", u, token, expires)
	}

	// Some titles carry a query on the playlist URL.
	htmlQuery := []byte(`
        window.masterPlaylist = {
            params: { 'token': 'abc', 'expires': '123', 'asn': '', },
            url: 'https://vixsrc.to/playlist/777603?b=1',
        }
`)
	u, _, _, err = vixsrcParseMasterPlaylist(htmlQuery)
	if err != nil || u != "https://vixsrc.to/playlist/777603?b=1" {
		t.Fatalf("query playlist parse failed: %q err=%v", u, err)
	}

	if _, _, _, err := vixsrcParseMasterPlaylist([]byte(`<html></html>`)); err == nil {
		t.Fatal("missing metadata accepted")
	}
}

func TestVixSrcMasterURL(t *testing.T) {
	got := vixsrcMasterURL("https://vixsrc.to/playlist/234272", "a b", "123")
	if !strings.HasPrefix(got, "https://vixsrc.to/playlist/234272?") {
		t.Fatalf("bad base: %q", got)
	}
	for _, want := range []string{"token=a+b", "expires=123", "asn=", "h=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}

	// Existing query parameters (e.g. ?b=1) must be preserved with the
	// required flags appended.
	got = vixsrcMasterURL("https://vixsrc.to/playlist/777603?b=1", "tok", "42")
	for _, want := range []string{"b=1", "token=tok", "expires=42", "h=1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "?b=1?") {
		t.Fatalf("malformed double query: %q", got)
	}
}

func TestVixSrcFirstVariant(t *testing.T) {
	master := `#EXTM3U
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",NAME="Italian",DEFAULT=YES,URI="https://vixsrc.to/playlist/1?type=audio&rendition=ita"
#EXT-X-STREAM-INF:BANDWIDTH=1200000,RESOLUTION=854x480,AUDIO="audio"
https://vixsrc.to/playlist/1?type=video&rendition=480p&token=a
#EXT-X-STREAM-INF:BANDWIDTH=4500000,RESOLUTION=1920x1080,AUDIO="audio"
https://vixsrc.to/playlist/1?type=video&rendition=1080p&token=b
`
	variant, err := vixsrcFirstVariant(master)
	if err != nil {
		t.Fatal(err)
	}
	if variant != "https://vixsrc.to/playlist/1?type=video&rendition=480p&token=a" {
		t.Fatalf("unexpected variant: %q", variant)
	}
	if _, err := vixsrcFirstVariant("#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,URI=\"https://x\"\n"); err == nil {
		t.Fatal("master without video variants accepted")
	}
}

func TestVixSrcFetchURLGuards(t *testing.T) {
	client := &http.Client{}
	for _, bad := range []string{
		"ftp://vixsrc.to/x",
		"https://user:pass@vixsrc.to/x",
		"http://vixsrc.to/x",
		"https://localhost/x",
		"https://127.0.0.1/x",
		"https://x.local/x",
	} {
		if _, err := vixsrcFetch(context.Background(), client, bad, 1024); err == nil {
			t.Errorf("URL %q was accepted", bad)
		}
	}
}
