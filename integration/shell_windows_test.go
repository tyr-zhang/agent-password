//go:build windows

package integration_test

func shell(script string) []string { return []string{"cmd", "/c", script} }
