// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/packagemanifest"
)

var exitProcess = os.Exit

func main() {
	exitProcess(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("package-manifest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	root := flags.String("package-root", "", "final package root")
	verify := flags.Bool("verify", false, "verify an existing manifest without writing it")
	version := flags.String("version", "", "release version")
	commit := flags.String("commit", "", "release commit")
	edition := flags.String("edition", "", "release edition")
	goos := flags.String("goos", "", "target GOOS")
	goarch := flags.String("goarch", "", "target GOARCH")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: package-manifest [--verify] --package-root ROOT --version VERSION --commit COMMIT --edition EDITION --goos GOOS --goarch GOARCH")
	}
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 || *root == "" || *version == "" || *commit == "" || *edition == "" || *goos == "" || *goarch == "" {
		flags.Usage()
		return 2
	}
	if len(*commit) != 40 {
		fmt.Fprintln(stderr, "package-manifest: release commit must be exactly 40 lowercase hexadecimal characters")
		return 2
	}
	for _, character := range *commit {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			fmt.Fprintln(stderr, "package-manifest: release commit must be exactly 40 lowercase hexadecimal characters")
			return 2
		}
	}
	identity := packagemanifest.Identity{
		Release: packagemanifest.Release{Version: *version, Commit: *commit, Edition: *edition},
		Target:  packagemanifest.Target{GOOS: *goos, GOARCH: *goarch},
	}
	var err error
	if *verify {
		_, err = packagemanifest.VerifyTree(*root, identity)
	} else {
		var manifest packagemanifest.Manifest
		manifest, err = packagemanifest.Build(*root, identity)
		if err == nil {
			err = packagemanifest.WriteAtomic(*root, manifest)
		}
		if err == nil {
			_, err = packagemanifest.VerifyTree(*root, identity)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "package-manifest: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, packagemanifest.ManifestName)
	return 0
}
