package file

import "testing"

// Batch import builds its "already stored" set from DuplicateKeys. The old
// code inserted DuplicateKey(""), whose protocol component is empty, while an
// incoming http:// or socks5:// link is deduped with DuplicateKey("http") or
// DuplicateKey("socks5"). The two could never be equal, so re-importing an
// existing plain proxy created a duplicate instead of being skipped.
func TestDuplicateKeysMatchIncomingLinkKeys(t *testing.T) {
	node := NewProxyNode()
	node.Host = "1.2.3.4"
	node.Port = 8080
	node.Username = "User"
	node.Http = true
	node.Socks5 = true

	keys := node.DuplicateKeys()
	set := map[string]struct{}{}
	for _, k := range keys {
		set[k] = struct{}{}
	}
	if len(keys) != 2 {
		t.Fatalf("a HTTP+SOCKS5 node should occupy 2 keys, got %d (%v)", len(keys), keys)
	}

	// These are exactly the keys the importer computes for incoming links.
	for _, proto := range []string{"http", "socks5"} {
		incoming := duplicateKey(proto, "1.2.3.4", 8080, "User")
		if _, ok := set[incoming]; !ok {
			t.Fatalf("existing node does not match incoming %s link key %q; keys=%v", proto, incoming, keys)
		}
	}

	// The empty-protocol key that the buggy caller used must not be in the set.
	if _, ok := set[duplicateKey("", "1.2.3.4", 8080, "User")]; ok {
		t.Fatal("DuplicateKeys must not emit an empty-protocol key")
	}
}

// The dedupe key is case-insensitive on host and protocol but keeps the
// username verbatim, and it always carries host, port and username.
func TestDuplicateKeyComposition(t *testing.T) {
	got := duplicateKey("HTTP", " Example.COM ", 8080, "Bob")
	want := "http|example.com|8080|Bob"
	if got != want {
		t.Fatalf("duplicateKey = %q, want %q", got, want)
	}
}
