package ip

import "testing"

func TestValidateIPv4(t *testing.T) {
	t.Parallel()

	ip, err := Validate("93.184.216.34\n", 4)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if got, want := ip.String(), "93.184.216.34"; got != want {
		t.Fatalf("ip = %q, want %q", got, want)
	}
}

func TestValidateRejectsIPv4ForIPv6(t *testing.T) {
	t.Parallel()

	if _, err := Validate("93.184.216.34", 6); err == nil {
		t.Fatal("Validate returned nil error, want IPv6 validation error")
	}
}

func TestValidateIPv6(t *testing.T) {
	t.Parallel()

	ip, err := Validate("2001:db8::1234", 6)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if got, want := ip.String(), "2001:db8::1234"; got != want {
		t.Fatalf("ip = %q, want %q", got, want)
	}
}
