package config

import "testing"

func Test_sanitizeFileName(t *testing.T) {
	type args struct {
		input string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{"WithSpace", args{"San First"}, "san_first"},
		{"WirthNum", args{"San0First"}, "san0first"},
		{"CyrillicWithNum", args{"Тест МСе0"}, "0"},
		{"CyrillicWith", args{"Тест МСе"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeFileName(tt.args.input); got != tt.want {
				t.Errorf("sanitizeFileName() = %v, want %v", got, tt.want)
			}
		})
	}
}
