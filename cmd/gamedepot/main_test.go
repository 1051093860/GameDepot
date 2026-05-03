package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseConfigAddOSSArgsSupportsFlagsAfterName(t *testing.T) {
	name, region, bucket, endpoint, internal, accelerate, err := parseConfigAddOSSArgs([]string{
		"aliyun-oss",
		"--region", "cn-shenzhen",
		"--bucket", "lsq",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if name != "aliyun-oss" || region != "cn-shenzhen" || bucket != "lsq" {
		t.Fatalf("unexpected result: name=%q region=%q bucket=%q", name, region, bucket)
	}
	if endpoint != "" || internal || accelerate {
		t.Fatalf("unexpected endpoint/options: endpoint=%q internal=%v accelerate=%v", endpoint, internal, accelerate)
	}
}

func TestParseConfigAddOSSArgsSupportsFlagsBeforeName(t *testing.T) {
	name, region, bucket, _, _, _, err := parseConfigAddOSSArgs([]string{
		"--region", "cn-shenzhen",
		"--bucket", "lsq",
		"aliyun-oss",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if name != "aliyun-oss" || region != "cn-shenzhen" || bucket != "lsq" {
		t.Fatalf("unexpected result: name=%q region=%q bucket=%q", name, region, bucket)
	}
}

func TestParseConfigSetCredentialsSupportsFlagsAfterName(t *testing.T) {
	name, id, secret, err := parseConfigSetCredentialsArgs([]string{
		"aliyun-oss",
		"--access-key-id", "id",
		"--access-key-secret", "secret",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if name != "aliyun-oss" || id != "id" || secret != "secret" {
		t.Fatalf("unexpected result: name=%q id=%q secret=%q", name, id, secret)
	}
}

func TestOldTopLevelCommandsAreRemoved(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	for _, cmd := range []string{"pull", "sync", "submit", "push", "rules"} {
		os.Args = []string{"gamedepot", cmd}
		err := run()
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded", cmd)
		}
		if !strings.Contains(err.Error(), "removed") {
			t.Fatalf("%s error should mention removal, got %v", cmd, err)
		}
	}
}

func TestReorderFlagsAllowsFlagsAfterPositionals(t *testing.T) {
	got := reorderFlags([]string{".", "--remote", "https://example.com/repo.git", "--branch", "main", "--no-plugin"}, map[string]bool{"no-plugin": true})
	want := []string{"--remote", "https://example.com/repo.git", "--branch", "main", "--no-plugin", "."}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("reordered = %#v, want %#v", got, want)
	}
}
