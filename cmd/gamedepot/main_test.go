package main

import "testing"

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
