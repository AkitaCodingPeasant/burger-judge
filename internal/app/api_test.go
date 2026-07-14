package app

import "testing"

// TestServerURL 驗證監聽位址會轉換成可直接開啟的 HTTP 網址。
func TestServerURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{name: "default port", addr: ":8080", want: "http://localhost:8080"},
		{name: "explicit host", addr: "127.0.0.1:9090", want: "http://127.0.0.1:9090"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverURL(tt.addr); got != tt.want {
				t.Fatalf("serverURL(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}
