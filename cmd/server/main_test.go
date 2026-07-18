package main

import "testing"

func TestListenPortDefaultsTo8888(t *testing.T) {
	t.Setenv("PORT", "")

	if got := listenPort(); got != "8888" {
		t.Fatalf("listenPort() = %q, want %q", got, "8888")
	}
}

func TestListenPortUsesPortEnv(t *testing.T) {
	t.Setenv("PORT", "9000")

	if got := listenPort(); got != "9000" {
		t.Fatalf("listenPort() = %q, want %q", got, "9000")
	}
}

func TestListenAddrDefaultsToAllInterfacesOnPort8888(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("PORT", "")

	if got := listenAddr(); got != ":8888" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":8888")
	}
}

func TestListenAddrUsesPortEnvWhenAddressNotSet(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "")
	t.Setenv("PORT", "9000")

	if got := listenAddr(); got != ":9000" {
		t.Fatalf("listenAddr() = %q, want %q", got, ":9000")
	}
}

func TestListenAddrPrefersExplicitListenAddr(t *testing.T) {
	t.Setenv("LISTEN_ADDR", "127.0.0.1:8888")
	t.Setenv("PORT", "9000")

	if got := listenAddr(); got != "127.0.0.1:8888" {
		t.Fatalf("listenAddr() = %q, want %q", got, "127.0.0.1:8888")
	}
}
