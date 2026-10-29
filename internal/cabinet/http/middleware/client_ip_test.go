package middleware

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	cdn := []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}

	cases := []struct {
		name   string
		remote string
		xff    []string
		xri    string
		extra  []netip.Prefix
		want   string
	}{
		{name: "прямое подключение без прокси", remote: "198.51.100.7:4000", want: "198.51.100.7"},
		{name: "прямое подключение: XFF клиента игнорируется", remote: "198.51.100.7:4000", xff: []string{"6.6.6.6"}, want: "198.51.100.7"},
		{name: "nginx в docker-сети", remote: "172.18.0.2:5000", xff: []string{"1.2.3.4"}, want: "1.2.3.4"},
		// Главный случай: nginx с $proxy_add_x_forwarded_for дописывает настоящий
		// адрес к тому, что прислал клиент. Раньше брался левый — 6.6.6.6.
		{name: "подделанный левый элемент", remote: "172.18.0.2:5000", xff: []string{"6.6.6.6, 1.2.3.4"}, want: "1.2.3.4"},
		{name: "несколько заголовков XFF — один список", remote: "127.0.0.1:5000", xff: []string{"6.6.6.6", "1.2.3.4"}, want: "1.2.3.4"},
		{name: "CDN перед nginx, CDN в доверенных", remote: "172.18.0.2:5000", xff: []string{"6.6.6.6, 1.2.3.4, 203.0.113.10"}, extra: cdn, want: "1.2.3.4"},
		{name: "CDN перед nginx, CDN не в доверенных", remote: "172.18.0.2:5000", xff: []string{"1.2.3.4, 203.0.113.10"}, want: "203.0.113.10"},
		{name: "доверенный прокси вне частных сетей", remote: "203.0.113.5:443", xff: []string{"1.2.3.4"}, extra: cdn, want: "1.2.3.4"},
		{name: "клиент сам в частной сети", remote: "172.18.0.2:5000", xff: []string{"10.0.0.5"}, want: "10.0.0.5"},
		{name: "мусор справа — верим только прокси", remote: "172.18.0.2:5000", xff: []string{"1.2.3.4, garbage"}, want: "172.18.0.2"},
		{name: "мусор левее настоящего адреса не мешает", remote: "172.18.0.2:5000", xff: []string{"garbage, 1.2.3.4"}, want: "1.2.3.4"},
		{name: "адрес с портом", remote: "172.18.0.2:5000", xff: []string{"1.2.3.4:5678"}, want: "1.2.3.4"},
		{name: "IPv6", remote: "[::1]:5000", xff: []string{"2001:db8::1"}, want: "2001:db8::1"},
		{name: "IPv4-mapped IPv6", remote: "[::ffff:172.18.0.2]:5000", xff: []string{"::ffff:1.2.3.4"}, want: "1.2.3.4"},
		{name: "X-Real-IP, если XFF нет", remote: "172.18.0.2:5000", xri: "1.2.3.4", want: "1.2.3.4"},
		{name: "X-Real-IP от недоверенного источника игнорируется", remote: "198.51.100.7:4000", xri: "1.2.3.4", want: "198.51.100.7"},
		{name: "XFF важнее X-Real-IP", remote: "172.18.0.2:5000", xff: []string{"1.2.3.4"}, xri: "6.6.6.6", want: "1.2.3.4"},
		{name: "прокси без заголовков", remote: "172.18.0.2:5000", want: "172.18.0.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.xff {
				r.Header.Add("X-Forwarded-For", v)
			}
			if tc.xri != "" {
				r.Header.Set("X-Real-IP", tc.xri)
			}
			if got := clientIP(r, tc.extra); got != tc.want {
				t.Fatalf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}
