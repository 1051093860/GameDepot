package main

import "flag"

func moveLeadingNameToEnd(args []string) []string {
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		out := append([]string{}, args[1:]...)
		out = append(out, args[0])
		return out
	}
	return args
}

// Kept for older CLI parser tests; runConfig now parses inline.
func parseConfigAddOSSArgs(args []string) (name, region, bucket, endpoint string, internal, accelerate bool, err error) {
	fs := flag.NewFlagSet("config add-oss", flag.ContinueOnError)
	fs.StringVar(&region, "region", "cn-hangzhou", "")
	fs.StringVar(&bucket, "bucket", "", "")
	fs.StringVar(&endpoint, "endpoint", "", "")
	fs.BoolVar(&internal, "internal", false, "")
	fs.BoolVar(&accelerate, "accelerate", false, "")
	if err = fs.Parse(moveLeadingNameToEnd(args)); err != nil {
		return
	}
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	return
}

// Kept for older CLI parser tests; runConfig now parses inline.
func parseConfigSetCredentialsArgs(args []string) (name, accessKeyID, accessKeySecret string, err error) {
	fs := flag.NewFlagSet("config set-credentials", flag.ContinueOnError)
	fs.StringVar(&accessKeyID, "access-key-id", "", "")
	fs.StringVar(&accessKeySecret, "access-key-secret", "", "")
	if err = fs.Parse(moveLeadingNameToEnd(args)); err != nil {
		return
	}
	if fs.NArg() > 0 {
		name = fs.Arg(0)
	}
	return
}
