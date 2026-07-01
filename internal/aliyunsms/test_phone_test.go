package aliyunsms

import "testing"

func TestIsTestPhone(t *testing.T) {
	tests := []struct {
		phone string
		want  bool
	}{
		{"10012345678", true},
		{"10000000000", true},
		{"13812345678", false},
		{"1001234567", false},
		{"100123456789", false},
		{"20012345678", false},
		{"", false},
		{"1001234567a", false},
	}
	for _, tt := range tests {
		if got := IsTestPhone(tt.phone); got != tt.want {
			t.Errorf("IsTestPhone(%q) = %v, want %v", tt.phone, got, tt.want)
		}
	}
}

func TestSendVerifyCodeTestPhone(t *testing.T) {
	if err := SendVerifyCode(nil, SMSParams{}, "10012345678"); err != nil {
		t.Fatalf("SendVerifyCode test phone: %v", err)
	}
}

func TestCheckVerifyCodeTestPhone(t *testing.T) {
	ok, err := CheckVerifyCode(nil, SMSParams{}, "10012345678", TestSMSVerifyCode)
	if err != nil {
		t.Fatalf("CheckVerifyCode: %v", err)
	}
	if !ok {
		t.Fatal("expected pass for test code")
	}
	ok, err = CheckVerifyCode(nil, SMSParams{}, "10012345678", "1234")
	if err != nil {
		t.Fatalf("CheckVerifyCode wrong code: %v", err)
	}
	if ok {
		t.Fatal("expected fail for wrong code")
	}
}
