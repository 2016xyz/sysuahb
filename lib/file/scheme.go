package file

// Tunnel scheme names. Empty, "http" and "socks5" are the original plain
// proxies; everything else is a tunnel protocol that carries its own
// encryption and is dialed by the server on behalf of the unified proxy.
const (
	SchemeHTTP   = "http"
	SchemeSOCKS5 = "socks5"
	SchemeSS     = "ss"
	SchemeSSR    = "ssr"
	SchemeVMess  = "vmess"
	SchemeVLESS  = "vless"
	SchemeTrojan = "trojan"
	SchemeTUIC   = "tuic"
	SchemeHY2    = "hysteria2"
	SchemeNaive  = "naive"
	SchemeClash  = "clash"
)

// tunnelSchemes lists every scheme that is stored as a single tunnel node
// rather than as the plain HTTP/SOCKS5 flag pair.
var tunnelSchemes = map[string]struct{}{
	SchemeSS: {}, SchemeSSR: {}, SchemeVMess: {}, SchemeVLESS: {},
	SchemeTrojan: {}, SchemeTUIC: {}, SchemeHY2: {}, SchemeNaive: {},
	SchemeClash: {},
}

// IsTunnelScheme reports whether scheme names a tunnel protocol.
func IsTunnelScheme(scheme string) bool {
	_, ok := tunnelSchemes[scheme]
	return ok
}

// SchemeLabel returns the display name of a scheme.
func SchemeLabel(scheme string) string {
	switch scheme {
	case SchemeSS:
		return "SS"
	case SchemeSSR:
		return "SSR"
	case SchemeVMess:
		return "VMess"
	case SchemeVLESS:
		return "VLESS"
	case SchemeTrojan:
		return "Trojan"
	case SchemeTUIC:
		return "TUIC"
	case SchemeHY2:
		return "Hysteria2"
	case SchemeNaive:
		return "NaiveProxy"
	case SchemeClash:
		return "Clash"
	case SchemeSOCKS5:
		return "SOCKS5"
	case SchemeHTTP, "":
		return "HTTP"
	default:
		return scheme
	}
}
